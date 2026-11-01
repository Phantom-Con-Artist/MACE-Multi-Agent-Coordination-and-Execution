package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// The job board: work that needs an owner, as a status is something an
// agent needs. The lead posts jobs with what they need; agents claim the
// ones their machines suit, checked against the registry; an owner reports
// progress, passes a job on, drops it, or marks it done, and the lead
// approves. Jobs sync to linked machines in the same session. See
// docs/DESIGN-COMPANY.md.

const (
	jobsPerGroup  = 12
	jobsPerAgent  = 2
	jobBoardLimit = 8
)

var (
	jobPostPattern   = regexp.MustCompile(`(?i)\[job:\s*([^\]\n]+)\]`)
	jobActionPattern = regexp.MustCompile(`(?i)\[(claim|pass|progress|done|drop|approve)\s+(j[0-9a-f]{4,8})(?:\s+@([A-Za-z0-9][A-Za-z0-9._@-]*[A-Za-z0-9]))?\]`)
	exposePattern    = regexp.MustCompile(`(?i)\[expose\s+(\d{2,5})\s+for\s+(j[0-9a-f]{4,8})\]`)
)

// Job is one piece of work on the board. Member IDs are node::session.
type Job struct {
	ID        string    `json:"id"`
	Home      string    `json:"home"`
	Session   string    `json:"session,omitempty"`
	Title     string    `json:"title"`
	Needs     []string  `json:"needs,omitempty"`
	Room      string    `json:"room"`
	RoomName  string    `json:"room_name"`
	Lead      string    `json:"lead,omitempty"`
	Poster    string    `json:"poster"`
	Owner     string    `json:"owner,omitempty"`
	OwnerName string    `json:"owner_name,omitempty"`
	Machine   string    `json:"machine,omitempty"`
	State     string    `json:"state"` // open, claimed, review, approved
	Progress  string    `json:"progress,omitempty"`
	Proof     string    `json:"proof,omitempty"`
	Services  []string  `json:"services,omitempty"`
	At        time.Time `json:"at"`
	Claimed   time.Time `json:"claimed,omitempty"`
	Updated   time.Time `json:"updated"`
}

func (j Job) active() bool { return j.State != "approved" }

func (j Job) needsGPU() bool {
	return slices.ContainsFunc(j.Needs, func(n string) bool { n = strings.ToLower(n); return n == "gpu" || n == "cuda" || n == "nvidia" })
}

func newJobID() string {
	b := make([]byte, 2)
	rand.Read(b)
	return "j" + hex.EncodeToString(b)
}

func (h *APIHandler) jobList(ctx context.Context) []Job {
	raw, _ := h.store.Documents(ctx, "job")
	out := []Job{}
	for _, b := range raw {
		var j Job
		if json.Unmarshal(b, &j) == nil && j.ID != "" {
			out = append(out, j)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].At.Before(out[k].At) })
	return out
}

func (h *APIHandler) jobGet(ctx context.Context, id string) (Job, bool) {
	var j Job
	return j, h.store.Document(ctx, "job", id, &j) == nil && j.ID != ""
}

// jobSave stores a job, tells the apps, and from its home machine sends it to
// every linked machine.
func (h *APIHandler) jobSave(ctx context.Context, j Job) {
	j.Updated = time.Now().UTC()
	h.statusMu.Lock()
	h.store.PutDocument(ctx, "job", j.ID, j)
	h.statusMu.Unlock()
	h.events.publish("job", j)
	if j.Home == h.selfID() {
		go func() {
			for _, n := range h.knownNodes(context.Background()) {
				if n.ID == h.selfID() || !h.nodeAvailable(n.ID) || !h.sameSession(n.ID) {
					continue
				}
				c, cancel := context.WithTimeout(context.Background(), 8*time.Second)
				if err := h.nodeRequest(c, n, "POST", "/api/v1/node/job", j, nil); err != nil {
					log.Printf("[MACE] Job %s to %s: %v", j.ID, n.Name, err)
				}
				cancel()
			}
		}()
	}
}

func (h *APIHandler) jobReceive(ctx context.Context, j Job) {
	if j.ID == "" || j.Home == h.selfID() || j.Session != h.sessionID(ctx) {
		return
	}
	h.statusMu.Lock()
	defer h.statusMu.Unlock()
	var old Job
	if h.store.Document(ctx, "job", j.ID, &old) == nil && !j.Updated.After(old.Updated) {
		return
	}
	h.store.PutDocument(ctx, "job", j.ID, j)
	h.events.publish("job", j)
}

func (h *APIHandler) jobPull(ctx context.Context, n Node) {
	var list []Job
	if h.nodeRequest(ctx, n, "GET", "/api/v1/node/jobs", nil, &list) == nil {
		for _, j := range list {
			h.jobReceive(ctx, j)
		}
	}
}

// nodeJob serves POST /api/v1/node/job: a linked machine's job.
func (h *APIHandler) nodeJob(w http.ResponseWriter, r *http.Request) {
	n, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var j Job
	if json.Unmarshal(body, &j) != nil || j.Home != n.ID {
		http.Error(w, "A machine can only send its own jobs.", 400)
		return
	}
	h.jobReceive(r.Context(), j)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// nodeJobs serves GET /api/v1/node/jobs: this machine's own jobs.
func (h *APIHandler) nodeJobs(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.verifyNodeRequest(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	out := []Job{}
	for _, j := range h.jobList(r.Context()) {
		if j.Home == h.selfID() {
			out = append(out, j)
		}
	}
	writeJSON(w, 200, out)
}

type jobAction struct {
	Job    string `json:"job"`
	Verb   string `json:"verb"`
	Member string `json:"member"`
	Target string `json:"target,omitempty"`
	Text   string `json:"text,omitempty"`
	// Facts are the acting machine's measured facts, for a claim or pass
	// checked on the job's home machine.
	Facts *MachineFacts `json:"facts,omitempty"`
	Port  int           `json:"port,omitempty"`
}

// nodeJobAction serves POST /api/v1/node/job-action for a job that lives here.
func (h *APIHandler) nodeJobAction(w http.ResponseWriter, r *http.Request) {
	n, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var a jobAction
	if json.Unmarshal(body, &a) != nil || !strings.HasPrefix(a.Member, n.ID+"::") {
		http.Error(w, "A machine can only act for its own agents.", 400)
		return
	}
	if a.Facts != nil {
		a.Facts.Node = n.ID // facts come from the machine that sent them
	}
	if err := h.jobApply(r.Context(), a); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// jobAct applies an action on the job's home machine, here or by forwarding.
func (h *APIHandler) jobAct(ctx context.Context, a jobAction) error {
	j, ok := h.jobGet(ctx, a.Job)
	if !ok {
		return fmt.Errorf("there is no job %s", a.Job)
	}
	if a.Verb == "claim" || a.Verb == "pass" {
		member := a.Member
		if a.Verb == "pass" {
			member = a.Target
		}
		if f, ok := h.factsOf(ctx, member); ok {
			a.Facts = &f
		}
	}
	if j.Home == h.selfID() {
		return h.jobApply(ctx, a)
	}
	for _, n := range h.knownNodes(ctx) {
		if n.ID == j.Home {
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return h.nodeRequest(c, n, "POST", "/api/v1/node/job-action", a, nil)
		}
	}
	return fmt.Errorf("the machine job %s came from is not linked", a.Job)
}

// gpuReservations maps each machine to the job using its GPU.
func (h *APIHandler) gpuReservations(ctx context.Context) map[string]string {
	busy := map[string]string{}
	for _, j := range h.jobList(ctx) {
		if (j.State == "claimed" || j.State == "review") && j.needsGPU() && j.Owner != "" {
			node, _, _ := strings.Cut(j.Owner, "::")
			busy[node] = j.ID
		}
	}
	return busy
}

// jobApply runs an action on the job's home machine.
func (h *APIHandler) jobApply(ctx context.Context, a jobAction) error {
	h.statusActMu.Lock()
	defer h.statusActMu.Unlock()
	j, ok := h.jobGet(ctx, a.Job)
	if !ok || j.Home != h.selfID() {
		return fmt.Errorf("job %s does not live on this machine", a.Job)
	}
	if !j.active() {
		return fmt.Errorf("job %s is already approved", j.ID)
	}
	owner := j.Owner == a.Member
	note := ""
	switch a.Verb {
	case "claim", "pass":
		who := a.Member
		if a.Verb == "pass" {
			if !owner {
				return fmt.Errorf("only the owner of job %s can pass it on", j.ID)
			}
			who = a.Target
		} else if j.Owner != "" && !owner {
			return fmt.Errorf("job %s already belongs to %s", j.ID, j.OwnerName)
		}
		if who == "" {
			return fmt.Errorf("pass job %s to whom?", j.ID)
		}
		if a.Facts == nil {
			return fmt.Errorf("MACE does not know that machine's resources yet; try again in a minute")
		}
		if miss := needsMet(*a.Facts, j.Needs); len(miss) > 0 {
			return fmt.Errorf("job %s needs %s, which %s does not have", j.ID, strings.Join(miss, ", "), a.Facts.Name)
		}
		if j.needsGPU() {
			if other, busy := h.gpuReservations(ctx)[a.Facts.Node]; busy && other != j.ID {
				return fmt.Errorf("the GPU on %s is busy with job %s", a.Facts.Name, other)
			}
		}
		held := 0
		for _, o := range h.jobList(ctx) {
			if o.Owner == who && o.ID != j.ID && (o.State == "claimed" || o.State == "review") {
				held++
			}
		}
		if held >= jobsPerAgent {
			return fmt.Errorf("that agent already has %d jobs", held)
		}
		j.Owner, j.OwnerName, j.Machine, j.State, j.Claimed, j.Progress = who, h.agentName(ctx, h.localize(who)), a.Facts.Name, "claimed", time.Now().UTC(), ""
		note = fmt.Sprintf("%s took job %s: %s.", j.OwnerName, j.ID, j.Title)
	case "progress", "done", "drop":
		if !owner {
			return fmt.Errorf("only the owner of job %s can do that", j.ID)
		}
		switch a.Verb {
		case "progress":
			j.Progress = limited(a.Text, 200)
		case "done":
			j.State, j.Proof = "review", limited(a.Text, 400)
			note = fmt.Sprintf("%s finished job %s; it waits for approval.", j.OwnerName, j.ID)
		case "drop":
			note = fmt.Sprintf("%s dropped job %s: %s", j.OwnerName, j.ID, limited(a.Text, 160))
			j.Owner, j.OwnerName, j.Machine, j.State, j.Services = "", "", "", "open", nil
		}
	case "approve":
		if a.Member != j.Lead && a.Member != j.Poster && a.Member != "you" {
			return fmt.Errorf("only the lead approves job %s", j.ID)
		}
		if j.State != "review" {
			return fmt.Errorf("job %s is not done yet", j.ID)
		}
		j.State, j.Services = "approved", nil
		note = fmt.Sprintf("Job %s is approved: %s.", j.ID, j.Title)
	case "expose":
		if !owner || a.Port == 0 {
			return fmt.Errorf("only the owner of job %s can expose a port for it", j.ID)
		}
		svc := h.serviceAddress(ctx, a.Member, a.Port)
		if !slices.Contains(j.Services, svc) {
			j.Services = append(j.Services, svc)
		}
		note = fmt.Sprintf("%s shared %s for job %s.", j.OwnerName, svc, j.ID)
	default:
		return fmt.Errorf("unknown job action %s", a.Verb)
	}
	h.jobSave(ctx, j)
	if note != "" {
		h.commandNote(j.Room, note, false)
	}
	return nil
}

// serviceAddress is where linked machines reach a port on a member's machine.
func (h *APIHandler) serviceAddress(ctx context.Context, member string, port int) string {
	node, _, remote := strings.Cut(h.localize(member), "::")
	host := ""
	if remote {
		for _, n := range h.knownNodes(ctx) {
			if n.ID == node {
				if u, err := url.Parse(n.URL); err == nil {
					host = u.Hostname()
				}
			}
		}
	} else if u, err := url.Parse(h.selfNode().URL); err == nil {
		host = u.Hostname()
	}
	if host == "" {
		host = "localhost"
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// jobPost records a new job from an agent in a group on this machine.
func (h *APIHandler) jobPost(ctx context.Context, group *Room, p Policy, poster, spec string) (Job, error) {
	title, needs, _ := strings.Cut(spec, "|")
	title = strings.TrimSpace(title)
	if title == "" {
		return Job{}, fmt.Errorf("say what the job is")
	}
	needs = strings.TrimSpace(needs)
	needs = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(needs), "needs:"))
	list := []string{}
	for _, n := range strings.FieldsFunc(needs, func(r rune) bool { return r == ',' || r == ' ' }) {
		if n = strings.TrimSpace(n); n != "" {
			list = append(list, n)
		}
	}
	open := 0
	for _, j := range h.jobList(ctx) {
		if j.Room == group.ID && j.active() {
			open++
		}
	}
	if open >= jobsPerGroup {
		return Job{}, fmt.Errorf("this group already has %d open jobs", jobsPerGroup)
	}
	lead := poster
	if p.Mode == "team" && p.Lead != "" {
		lead = p.Lead
	}
	now := time.Now().UTC()
	j := Job{ID: newJobID(), Home: h.selfID(), Session: h.sessionID(ctx), Title: limited(title, 200), Needs: list, Room: group.ID, RoomName: group.Name,
		Lead: h.qualify(lead), Poster: h.qualify(poster), State: "open", At: now}
	h.jobSave(ctx, j)
	return j, nil
}

// jobTags handles job tags in an agent's reply in a conversation on this
// machine, and returns the text without them.
func (h *APIHandler) jobTags(ctx context.Context, room string, group *Room, p Policy, members []string, member, text string) string {
	name := h.memberName(ctx, member)
	for _, m := range jobPostPattern.FindAllStringSubmatch(text, 6) {
		text = strings.Replace(text, m[0], "", 1)
		if group == nil {
			continue
		}
		if p.Mode == "team" && p.Lead != "" && member != p.Lead {
			h.commandNote(room, name+" tried to post a job; in a team only the lead posts them.", false)
			continue
		}
		if j, err := h.jobPost(ctx, group, p, member, m[1]); err != nil {
			h.commandNote(room, "Job not posted: "+err.Error(), false)
		} else {
			need := ""
			if len(j.Needs) > 0 {
				need = " (needs " + strings.Join(j.Needs, ", ") + ")"
			}
			h.commandNote(room, fmt.Sprintf("Job %s posted: %s%s.", j.ID, j.Title, need), false)
		}
	}
	for _, m := range exposePattern.FindAllStringSubmatch(text, 3) {
		text = strings.Replace(text, m[0], "", 1)
		port := 0
		fmt.Sscan(m[1], &port)
		if err := h.jobAct(ctx, jobAction{Job: strings.ToLower(m[2]), Verb: "expose", Member: h.qualify(member), Port: port}); err != nil {
			h.commandNote(room, name+": "+err.Error(), false)
		}
	}
	matches := jobActionPattern.FindAllStringSubmatch(text, 6)
	rest := strings.TrimSpace(jobActionPattern.ReplaceAllString(text, ""))
	for _, m := range matches {
		a := jobAction{Job: strings.ToLower(m[2]), Verb: strings.ToLower(m[1]), Member: h.qualify(member), Text: rest}
		if a.Verb == "pass" {
			a.Target = h.qualify(h.memberByHandle(ctx, members, m[3]))
		}
		if err := h.jobAct(ctx, a); err != nil {
			h.commandNote(room, name+": "+err.Error(), false)
		}
	}
	return strings.TrimSpace(jobActionPattern.ReplaceAllString(text, ""))
}

// memberByHandle finds an agent by handle: first in this conversation, then
// among every agent this machine knows.
func (h *APIHandler) memberByHandle(ctx context.Context, members []string, handle string) string {
	handle = strings.ToLower(handle)
	base := strings.SplitN(handle, "@", 2)[0]
	for id, hd := range h.roomHandles(ctx, members) {
		if strings.ToLower(hd) == handle {
			return id
		}
	}
	for _, kind := range []string{"session", "remote-session"} {
		raw, _ := h.store.Documents(ctx, kind)
		for _, b := range raw {
			var s Session
			if json.Unmarshal(b, &s) == nil && s.Status != "removed" && strings.ToLower(s.Handle) == base {
				return s.ID
			}
		}
	}
	return ""
}

// jobBoard is the awareness section an agent reads each turn: its own jobs,
// open jobs its machine can take, and, for a lead, every job of its teams.
func (h *APIHandler) jobBoard(ctx context.Context, s Session) string {
	self := h.qualify(s.ID)
	facts, known := h.factsOf(ctx, s.ID)
	var mine, open, team []string
	now := time.Now()
	for _, j := range h.jobList(ctx) {
		if !j.active() {
			continue
		}
		needs := ""
		if len(j.Needs) > 0 {
			needs = " [needs " + strings.Join(j.Needs, ", ") + "]"
		}
		switch {
		case j.Owner == self:
			line := fmt.Sprintf("- %s (%s): %s%s", j.ID, j.State, j.Title, needs)
			if len(j.Services) > 0 {
				line += " · shared: " + strings.Join(j.Services, " ")
			}
			mine = append(mine, line)
		case j.Lead == self:
			who := "unclaimed " + ago(now.Sub(j.At))
			if j.Owner != "" {
				who = j.OwnerName + " on " + j.Machine
				if j.Progress != "" {
					who += ": " + j.Progress
				}
				if w := h.healthWarnings(ctx, j.Owner); len(w) > 0 {
					who += " (at risk: " + strings.Join(w, "; ") + ")"
				}
			}
			team = append(team, fmt.Sprintf("- %s (%s, %s): %s%s", j.ID, j.State, who, j.Title, needs))
		case j.State == "open" && known && len(needsMet(facts, j.Needs)) == 0 && len(open) < jobBoardLimit:
			open = append(open, fmt.Sprintf("- %s: %s%s (group %q, %s old)", j.ID, j.Title, needs, limited(j.RoomName, 60), ago(now.Sub(j.At))))
		}
	}
	var b strings.Builder
	if len(mine) > 0 {
		b.WriteString("YOUR JOBS:\n" + strings.Join(mine, "\n") + "\n")
	}
	if len(team) > 0 {
		b.WriteString("JOBS YOU LEAD (approve finished ones with [approve <id>]; restaff unclaimed ones by mentioning who fits):\n" + strings.Join(team, "\n") + "\n")
	}
	if len(open) > 0 {
		b.WriteString("OPEN JOBS YOUR MACHINE CAN TAKE (claim one with [claim <id>] only if you will do it now):\n" + strings.Join(open, "\n") + "\n")
	}
	return b.String()
}

// jobs serves GET /api/v1/jobs to the owner's apps.
func (h *APIHandler) jobs(w http.ResponseWriter, r *http.Request) {
	out := []Job{}
	for _, j := range h.jobList(r.Context()) {
		if j.active() || time.Since(j.Updated) < 24*time.Hour {
			out = append(out, j)
		}
	}
	writeJSON(w, 200, out)
}
