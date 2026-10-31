package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Conversations are scheduled per agent. Each coworker has one inbox and works
// through it one turn at a time, because its native session can only do one
// thing at once. Different coworkers work in parallel. A turn request names
// the conversation it is for; the agent reads everything new there.
//
// Who gets a turn (protocol v3):
//   - Owner message, no mentions: every available member, in parallel.
//   - Owner message with @handles: only those members. @all means everyone.
//   - Agent message: only the members it @mentions. In continuous mode, every
//     other member also gets a paced, low-priority turn.
//   - Chains of agent-triggered turns stop after hopLimit, except in
//     continuous mode, which runs until paused.

const hopLimit = 8

// Priorities: lower runs first.
const (
	priorityDirect = iota
	priorityOwner
	priorityMention
	priorityContinuous
)

var (
	errGoalComplete = errors.New("goal reported complete")
	errLoop         = errors.New("conversation loop detected")
)

func continuousGap() time.Duration {
	if v, err := strconv.ParseFloat(os.Getenv("MACE_CONTINUOUS_GAP_SECONDS"), 64); err == nil && v >= 0 {
		return time.Duration(v * float64(time.Second))
	}
	return 20 * time.Second
}

type turnRequest struct {
	Member string
	Room   string
	// Gen is the owner message generation the turn belongs to. Each owner
	// message starts a new budget; turns still running from an earlier
	// message do not count against it.
	Gen       int
	Hop       int
	Priority  int
	NotBefore time.Time
	Created   time.Time
}

type memberQueue struct {
	queue     []*turnRequest
	running   *turnRequest
	cancel    context.CancelCauseFunc
	active    bool
	wake      chan struct{}
	lastStart map[string]time.Time
}

type roomRun struct {
	run      Run
	gen      int
	inflight int
	limitHit bool
	closed   bool
	// progress records a posted reply since the last continuous round.
	progress bool
	lastErr  string
	recent   []string
	strikes  int
}

// queueFor returns the member's inbox. The caller holds h.sched.
func (h *APIHandler) queueFor(member string) *memberQueue {
	q := h.queues[member]
	if q == nil {
		q = &memberQueue{wake: make(chan struct{}, 1), lastStart: map[string]time.Time{}}
		h.queues[member] = q
	}
	return q
}

// roomFor returns the run state of a conversation. The caller holds h.sched.
func (h *APIHandler) roomFor(room string) *roomRun {
	rr := h.roomRuns[room]
	if rr == nil {
		rr = &roomRun{run: Run{ID: room, Status: "idle"}}
		h.store.Document(context.Background(), "run", room, &rr.run)
		h.roomRuns[room] = rr
	}
	return rr
}

func (h *APIHandler) saveRun(room string) {
	// Computing and writing under one lock keeps saves in order; otherwise an
	// older snapshot written late could overwrite a newer one.
	h.runSaveMu.Lock()
	defer h.runSaveMu.Unlock()
	h.sched.Lock()
	rr := h.roomFor(room)
	rr.run.Updated = time.Now().UTC()
	rr.run.ActiveMembers = h.activeIn(room)
	rr.run.ActiveMember = ""
	if len(rr.run.ActiveMembers) > 0 {
		rr.run.ActiveMember = rr.run.ActiveMembers[0]
	}
	run := rr.run
	h.sched.Unlock()
	h.store.PutDocument(context.Background(), "run", room, run)
	h.events.publish("run", run)
}

// activeIn lists members currently working in a room. The caller holds h.sched.
func (h *APIHandler) activeIn(room string) []string {
	out := []string{}
	for member, q := range h.queues {
		if q.running != nil && q.running.Room == room {
			out = append(out, member)
		}
	}
	slices.Sort(out)
	return out
}

// pending reports whether a room has queued or running turns. The caller holds h.sched.
func (h *APIHandler) pending(room string) bool {
	for _, q := range h.queues {
		if q.running != nil && q.running.Room == room {
			return true
		}
		for _, r := range q.queue {
			if r.Room == room {
				return true
			}
		}
	}
	return false
}

func (h *APIHandler) enqueue(req *turnRequest) {
	q := h.queueFor(req.Member)
	for _, existing := range q.queue {
		if existing.Room == req.Room {
			// One queued turn per conversation is enough: the agent reads every
			// new message when it starts.
			existing.Priority = min(existing.Priority, req.Priority)
			existing.Hop = min(existing.Hop, req.Hop)
			existing.Gen = max(existing.Gen, req.Gen)
			if req.Priority < priorityContinuous {
				existing.NotBefore = time.Time{}
			}
			return
		}
	}
	if req.Priority == priorityContinuous {
		if last, ok := q.lastStart[req.Room]; ok {
			req.NotBefore = last.Add(continuousGap())
		}
	}
	q.queue = append(q.queue, req)
	if !q.active {
		q.active = true
		go h.work(req.Member)
		return
	}
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// work runs one member's inbox until it is empty.
func (h *APIHandler) work(member string) {
	for {
		h.sched.Lock()
		q := h.queues[member]
		now := time.Now()
		pick, wait := -1, time.Duration(0)
		for i, r := range q.queue {
			if r.NotBefore.After(now) {
				if d := r.NotBefore.Sub(now); wait == 0 || d < wait {
					wait = d
				}
				continue
			}
			if pick < 0 || r.Priority < q.queue[pick].Priority || r.Priority == q.queue[pick].Priority && r.Created.Before(q.queue[pick].Created) {
				pick = i
			}
		}
		if pick < 0 {
			if len(q.queue) == 0 {
				q.active = false
				h.sched.Unlock()
				return
			}
			h.sched.Unlock()
			select {
			case <-q.wake:
			case <-time.After(wait):
			}
			continue
		}
		req := q.queue[pick]
		q.queue = slices.Delete(q.queue, pick, pick+1)
		ctx, cancel := context.WithCancelCause(context.Background())
		q.running, q.cancel = req, cancel
		q.lastStart[req.Room] = now
		h.sched.Unlock()
		h.saveRun(req.Room)
		h.execute(ctx, req)
		cancel(nil)
		h.sched.Lock()
		q.running, q.cancel = nil, nil
		h.sched.Unlock()
		h.settle(req.Room)
	}
}

// settle finishes a run once nothing is queued or running for its room. A
// continuous conversation instead starts another round, unless a reply failed
// or every agent passed.
func (h *APIHandler) settle(room string) {
	h.sched.Lock()
	rr := h.roomFor(room)
	if h.pending(room) || rr.run.Status != "running" {
		h.sched.Unlock()
		h.saveRun(room)
		return
	}
	if rr.run.Policy.Mode == "continuous" && !rr.closed && rr.lastErr == "" {
		if rr.progress {
			rr.progress = false
			gen := rr.gen
			h.sched.Unlock()
			if h.continueRound(room, gen) {
				return
			}
			h.sched.Lock()
			rr.run.Status, rr.run.Error = "paused", "No agents are available. Resume an agent or add a member to continue."
			h.sched.Unlock()
			h.saveRun(room)
			return
		}
		rr.run.Status = "completed"
		h.sched.Unlock()
		h.systemNote(room, "Everyone passed, so the team went quiet. Write to get them going again.")
		h.saveRun(room)
		return
	}
	switch {
	case rr.limitHit:
		rr.run.Status = "limit_reached"
	case rr.lastErr != "" && (rr.run.Replies == 0 || rr.run.Policy.Mode == "continuous"):
		rr.run.Status, rr.run.Error = "failed", rr.lastErr
	default:
		rr.run.Status = "completed"
	}
	h.sched.Unlock()
	h.saveRun(room)
	h.audit("orchestrator", "CONVERSATION_"+strings.ToUpper(rr.run.Status), room, map[string]any{"replies": rr.run.Replies})
}

// halt stops a room: queued turns are dropped and running turns cancelled with
// cause. It reports whether any work was stopped.
func (h *APIHandler) halt(room string, cause error, status string) bool {
	h.sched.Lock()
	stopped := false
	for _, q := range h.queues {
		before := len(q.queue)
		q.queue = slices.DeleteFunc(q.queue, func(r *turnRequest) bool { return r.Room == room })
		stopped = stopped || len(q.queue) != before
		if q.running != nil && q.running.Room == room {
			q.cancel(cause)
			stopped = true
		}
	}
	rr := h.roomFor(room)
	wasRunning := rr.run.Status == "running"
	if wasRunning || stopped {
		rr.run.Status, rr.run.Error, rr.closed = status, "", true
	}
	h.sched.Unlock()
	if !(wasRunning || stopped) {
		return false
	}
	if why := interruption(cause); why != "" {
		h.sched.Lock()
		rr.run.Status, rr.run.Error = "interrupted", why
		h.sched.Unlock()
		h.systemNote(room, why)
	}
	h.saveRun(room)
	return true
}

// pauseRoom is the owner's pause, or an interruption when cause says so.
func (h *APIHandler) pauseRoom(room string, cause error) bool { return h.halt(room, cause, "paused") }

// stopAllRuns halts every room with work. Turns requested by connected hosts
// are not in these queues; stopWork handles their processes.
func (h *APIHandler) stopAllRuns(cause error) int {
	h.sched.Lock()
	rooms := []string{}
	for room, rr := range h.roomRuns {
		if rr.run.Status == "running" || h.pending(room) {
			rooms = append(rooms, room)
		}
	}
	h.sched.Unlock()
	n := 0
	for _, room := range rooms {
		if h.pauseRoom(room, cause) {
			n++
		}
	}
	return n
}

// stopMember cancels a member's running turn and clears its inbox everywhere.
func (h *APIHandler) stopMember(member string) {
	h.sched.Lock()
	rooms := []string{}
	if q := h.queues[member]; q != nil {
		for _, r := range q.queue {
			rooms = append(rooms, r.Room)
		}
		q.queue = nil
		if q.running != nil {
			q.cancel(nil)
		}
	}
	h.sched.Unlock()
	for _, room := range rooms {
		h.settle(room)
	}
}

// removeFromRoom drops a member's work for one room.
func (h *APIHandler) removeFromRoom(room, member string) {
	h.sched.Lock()
	if q := h.queues[member]; q != nil {
		q.queue = slices.DeleteFunc(q.queue, func(r *turnRequest) bool { return r.Room == room })
		if q.running != nil && q.running.Room == room {
			q.cancel(nil)
		}
	}
	h.sched.Unlock()
	h.settle(room)
}

// conversation returns a room's members and whether it is a group.
func (h *APIHandler) conversation(ctx context.Context, room string) (members []string, group *Room, err error) {
	var g Room
	if h.store.Document(ctx, "room", room, &g) == nil {
		if g.Removed {
			return nil, nil, errRemoved
		}
		return g.Members, &g, nil
	}
	if h.knownMember(ctx, room) {
		return []string{room}, nil, nil
	}
	return nil, nil, errNotFound
}

var (
	errRemoved  = errors.New("This group was removed.")
	errNotFound = errors.New("conversation not found")
)

func (h *APIHandler) policyFor(ctx context.Context, room string, members []string) Policy {
	p := Policy{Mode: "reply", Limit: max(len(members), 1)}
	h.store.Document(ctx, "policy", room, &p)
	return p
}

// dispatch decides who responds to a newly posted message and queues their turns.
func (h *APIHandler) dispatch(ctx context.Context, room string, m Message) error {
	members, group, err := h.conversation(ctx, room)
	if err != nil {
		return err
	}
	p := h.policyFor(ctx, room, members)
	owner := m.Sender == "you" || m.Wake
	targets := []string{}
	priority := priorityMention
	if owner {
		priority = priorityOwner
		if group == nil {
			priority = priorityDirect
		}
		if len(m.Mentions) > 0 {
			targets = m.Mentions
		} else {
			targets = members
		}
	} else {
		if m.Hop >= hopLimit && p.Mode != "continuous" {
			h.systemNote(room, fmt.Sprintf("Agents handed the conversation to each other %d times in a row, so Mace stopped the chain here. Reply to keep going.", hopLimit))
			return nil
		}
		targets = append(targets, m.Mentions...)
		if m.MentionsAll {
			targets = members
		}
	}
	type pick struct {
		member   string
		priority int
	}
	picks := []pick{}
	seen := map[string]bool{}
	for _, id := range targets {
		if id != m.Sender && slices.Contains(members, id) && !seen[id] && h.available(ctx, id) {
			seen[id] = true
			picks = append(picks, pick{id, priority})
		}
	}
	if !owner && p.Mode == "continuous" {
		others := []string{}
		for _, id := range members {
			if id != m.Sender && h.available(ctx, id) {
				others = append(others, id)
			}
		}
		// A lone member keeps the conversation going by itself.
		if len(others) == 0 && h.available(ctx, m.Sender) {
			others = []string{m.Sender}
		}
		for _, id := range others {
			if !seen[id] {
				seen[id] = true
				picks = append(picks, pick{id, priorityContinuous})
			}
		}
	}
	h.sched.Lock()
	rr := h.roomFor(room)
	if owner {
		// Each owner message starts a fresh reply budget.
		rr.run = Run{ID: room, Status: "running", Policy: p}
		rr.gen++
		rr.inflight, rr.limitHit, rr.closed, rr.lastErr, rr.recent, rr.strikes = 0, false, false, "", nil, 0
	} else if rr.closed {
		h.sched.Unlock()
		return nil
	}
	if len(picks) > 0 {
		rr.run.Status, rr.run.Policy = "running", p
	}
	now := time.Now()
	for _, c := range picks {
		h.enqueue(&turnRequest{Member: c.member, Room: room, Gen: rr.gen, Hop: m.Hop, Priority: c.priority, Created: now})
	}
	h.sched.Unlock()
	if owner && len(picks) == 0 {
		h.sched.Lock()
		rr.run.Status, rr.run.Error = "paused", "No agents are available. Resume an agent or add a member to continue."
		h.sched.Unlock()
		h.saveRun(room)
		return errors.New("no agents are available; resume an agent or add a member")
	}
	h.saveRun(room)
	return nil
}

// continueRound queues a paced turn for every available member of a
// continuous conversation. It reports whether anyone was queued.
func (h *APIHandler) continueRound(room string, gen int) bool {
	ctx := context.Background()
	members, _, err := h.conversation(ctx, room)
	if err != nil {
		return false
	}
	h.sched.Lock()
	defer h.sched.Unlock()
	queued := false
	now := time.Now()
	for _, id := range members {
		if h.available(ctx, id) {
			h.enqueue(&turnRequest{Member: id, Room: room, Gen: gen, Priority: priorityContinuous, Created: now})
			queued = true
		}
	}
	return queued
}

var passOnly = regexp.MustCompile(`(?i)^\s*\[pass\]\s*\.?\s*$`)

// execute runs one turn and posts the reply.
func (h *APIHandler) execute(ctx context.Context, req *turnRequest) {
	members, group, err := h.conversation(ctx, req.Room)
	if err != nil || (group != nil && !slices.Contains(members, req.Member)) || !h.available(ctx, req.Member) {
		return
	}
	p := h.policyFor(ctx, req.Room, members)
	h.sched.Lock()
	rr := h.roomFor(req.Room)
	current := req.Gen == rr.gen
	if rr.closed && current || current && p.Mode != "continuous" && rr.run.Replies+rr.inflight >= p.Limit {
		if !rr.closed {
			rr.limitHit = true
		}
		h.sched.Unlock()
		return
	}
	if current {
		rr.inflight++
	}
	h.sched.Unlock()
	posted := false
	defer func() {
		h.sched.Lock()
		// A newer owner message reset the budget; this turn no longer counts.
		if current && req.Gen == rr.gen && !posted {
			rr.inflight--
		}
		h.sched.Unlock()
	}()

	prompts, lastSeq, fresh := h.buildPrompts(ctx, req, members, group, p)
	// Nothing new from anyone else since this agent last looked: running it
	// again would only repeat itself. Continuous turns are exempt so a lone
	// member can keep working.
	if !fresh && req.Priority != priorityContinuous {
		return
	}
	live := func(steps []Activity) {
		h.events.publish("activity", map[string]any{"room": req.Room, "member": req.Member, "steps": steps})
	}
	live([]Activity{})
	var res turnResult
	if peer, id, remote := strings.Cut(req.Member, "::"); remote {
		var links string
		if links, err = h.shareFiles(ctx, req.Room, peer); err != nil {
			h.systemNote(req.Room, h.memberName(ctx, req.Member)+" could not receive an attached file: "+limited(err.Error(), 400))
			return
		}
		prompts.Full += links
		if prompts.Delta != "" {
			prompts.Delta += links
		}
		res, err = h.peerTurn(ctx, peer, id, prompts)
	} else {
		res, err = h.turn(ctx, req.Member, prompts, "local", live)
	}
	h.events.publish("activity", map[string]any{"room": req.Room, "member": req.Member, "done": true})
	if ctx.Err() != nil {
		return
	}
	name := h.memberName(ctx, req.Member)
	if err != nil {
		h.sched.Lock()
		rr.lastErr = err.Error()
		h.sched.Unlock()
		h.systemNote(req.Room, name+" could not reply: "+limited(err.Error(), 600))
		return
	}
	h.setCursor(ctx, req.Member, req.Room, lastSeq)
	if res.Reset {
		h.systemNote(req.Room, name+" started a fresh native session, so it may not remember earlier work in detail.")
	}
	if passOnly.MatchString(res.Text) {
		return
	}
	text := res.Text
	goal := p.Mode == "goal" && strings.Contains(text, "[GOAL_COMPLETE]")
	if goal {
		text = strings.TrimSpace(strings.ReplaceAll(text, "[GOAL_COMPLETE]", ""))
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, "[PASS]", ""))
	reply := Message{ID: newID(), Room: req.Room, Sender: req.Member, Text: text, At: time.Now().UTC(), Delivery: map[string]string{}, Hop: req.Hop + 1, Activity: res.Activity}
	reply.Mentions, reply.MentionsAll = parseMentions(text, h.roomHandles(ctx, members))
	if _, err := h.postMessage(ctx, &reply); err != nil {
		return
	}
	// Count the reply as soon as it is visible, before anything saves the run.
	posted = true
	h.sched.Lock()
	rr.progress = true
	if current && req.Gen == rr.gen {
		rr.inflight--
		rr.run.Replies++
	}
	h.sched.Unlock()
	if goal {
		h.halt(req.Room, errGoalComplete, "goal_reported_complete")
		return
	}
	if h.looping(req.Room, text) {
		h.systemNote(req.Room, "The agents kept repeating each other, so Mace paused this conversation. Reply to give them a new direction.")
		h.halt(req.Room, errLoop, "paused")
		return
	}
	h.dispatch(ctx, req.Room, reply)
}

// looping reports three near-identical replies in a row.
func (h *APIHandler) looping(room, text string) bool {
	words := strings.Fields(strings.ToLower(text))
	norm := strings.Join(words, " ")
	h.sched.Lock()
	defer h.sched.Unlock()
	rr := h.roomFor(room)
	similar := false
	for _, prev := range rr.recent {
		if jaccard(prev, norm) >= 0.9 {
			similar = true
		}
	}
	if similar {
		rr.strikes++
	} else {
		rr.strikes = 0
	}
	rr.recent = append(rr.recent, norm)
	if len(rr.recent) > 4 {
		rr.recent = rr.recent[1:]
	}
	return rr.strikes >= 2
}

func jaccard(a, b string) float64 {
	set := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, w := range strings.Fields(s) {
			m[w] = true
		}
		return m
	}
	x, y := set(a), set(b)
	if len(x) == 0 && len(y) == 0 {
		return 1
	}
	inter := 0
	for w := range x {
		if y[w] {
			inter++
		}
	}
	return float64(inter) / float64(len(x)+len(y)-inter)
}

func (h *APIHandler) setCursor(ctx context.Context, member, room string, seq int64) {
	if seq > 0 {
		h.store.PutDocument(ctx, "cursor", member+"|"+room, map[string]int64{"seq": seq})
		h.events.publish("cursor", map[string]any{"room": room, "member": member, "seq": seq})
	}
}

func (h *APIHandler) cursor(ctx context.Context, member, room string) int64 {
	var c struct {
		Seq int64 `json:"seq"`
	}
	h.store.Document(ctx, "cursor", member+"|"+room, &c)
	return c.Seq
}

// postMessage stores a message and announces it to connected browsers.
func (h *APIHandler) postMessage(ctx context.Context, m *Message) (int64, error) {
	seq, err := h.store.AppendMessage(ctx, m.Room, m.ID, m)
	if err != nil {
		return 0, err
	}
	m.Seq = seq
	h.events.publish("message", m)
	return seq, nil
}

func (h *APIHandler) updateMessage(ctx context.Context, m *Message) error {
	if err := h.store.UpdateMessage(ctx, m.ID, m); err != nil {
		return err
	}
	h.events.publish("message", m)
	return nil
}

// systemNote posts a visible Mace note that does not wake any agent.
func (h *APIHandler) systemNote(room, text string) {
	note := Message{ID: newID(), Room: room, Sender: "mace", Kind: "system", Text: text, At: time.Now().UTC(), Delivery: map[string]string{}}
	h.postMessage(context.Background(), &note)
}

// wakeAll posts a Mace message that every member answers, used to start a
// continuous discussion or continue after an interruption.
func (h *APIHandler) wakeAll(ctx context.Context, room, text string) (Message, error) {
	m := Message{ID: newID(), Room: room, Sender: "mace", Kind: "system", Wake: true, Text: text, At: time.Now().UTC(), Delivery: map[string]string{}}
	if _, err := h.postMessage(ctx, &m); err != nil {
		return m, err
	}
	return m, h.dispatch(ctx, room, m)
}

// shareFiles copies files attached in a conversation to a connected host once,
// and returns prompt lines naming the copies on that host.
func (h *APIHandler) shareFiles(ctx context.Context, room, peer string) (string, error) {
	rows, _, err := h.store.Messages(ctx, room, 0, 0, 40)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Data, &m) != nil || m.File == "" {
			continue
		}
		key := peer + ":" + m.File
		var link FileLink
		if h.store.Document(ctx, "filelinks:"+room, key, &link) == nil {
			continue
		}
		dest, err := h.transfer(ctx, peer, m.File)
		if err != nil {
			return "", err
		}
		if err := h.store.PutDocument(ctx, "filelinks:"+room, key, FileLink{Peer: peer, Source: m.File, Destination: dest}); err != nil {
			return "", err
		}
	}
	var lines strings.Builder
	links, _ := h.store.Documents(ctx, "filelinks:"+room)
	for _, b := range links {
		var link FileLink
		if json.Unmarshal(b, &link) == nil && link.Peer == peer {
			lines.WriteString("\nUse your local copy of " + link.Source + ": " + link.Destination)
		}
	}
	return lines.String(), nil
}
