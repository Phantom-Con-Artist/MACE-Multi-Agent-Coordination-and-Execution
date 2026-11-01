package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// The status board lets an agent that needs something its machine lacks ask
// every active agent at once, on every linked machine. In a team only the
// lead posts; agents read the board on their next turn, so a status costs no
// extra model calls. An agent that has the thing offers it, which opens a
// side chat with the requester; the requester picks one. See
// docs/DESIGN-STATUS.md.

const (
	statusLife        = 24 * time.Hour
	statusPerGroup    = 3
	statusOffersLimit = 5
	statusBoardLimit  = 8
)

var (
	statusPostPattern = regexp.MustCompile(`(?i)\[status:\s*([^\]\n]+)\]`)
	statusForPattern  = regexp.MustCompile(`(?i)\bfor\s+@([A-Za-z0-9][A-Za-z0-9._@-]*[A-Za-z0-9])\s*$`)
	offerPattern      = regexp.MustCompile(`(?i)\[offer\s+(s[0-9a-f]{4,12})\]\s*`)
	statusPickPattern = regexp.MustCompile(`(?i)\[status\s+pick\s+@([A-Za-z0-9][A-Za-z0-9._@-]*[A-Za-z0-9])\]`)
	statusDonePattern = regexp.MustCompile(`(?i)\[status\s+done\]`)
	// An agent that names an open status and says it can help, without the
	// [offer] tag, is offering; one that says it cannot is not.
	statusIDPattern = regexp.MustCompile(`\bs[0-9a-f]{6}\b`)
	offerWords      = regexp.MustCompile(`(?i)\b(i have|i've got|i got|got it|have it|can (take|do|help|run|share|give|send|provide|grab)|i'll (take|do|run|send|share|grab)|here(?:'s| is| are)|ready|on it)\b`)
	refusalWords    = regexp.MustCompile(`(?i)\b(can't|cannot|can not|unable|don't have|do not have|no access|not able|isn't|is down|shell's down)\b`)
)

// Status is one request on the board. Member IDs are qualified with the
// node they live on (node::session) so every machine reads them the same way.
type Status struct {
	ID       string        `json:"id"`
	Home     string        `json:"home"`
	Text     string        `json:"text"`
	For      string        `json:"for"`
	ForName  string        `json:"for_name"`
	Machine  string        `json:"machine"`
	Poster   string        `json:"poster"`
	Room     string        `json:"room"`
	RoomName string        `json:"room_name"`
	At       time.Time     `json:"at"`
	Expires  time.Time     `json:"expires"`
	Updated  time.Time     `json:"updated"`
	Closed   bool          `json:"closed,omitempty"`
	Picked   string        `json:"picked,omitempty"`
	Offers   []StatusOffer `json:"offers,omitempty"`
}

// StatusOffer is one agent's answer to a status, talked over in its side chat.
type StatusOffer struct {
	Member  string    `json:"member"`
	Name    string    `json:"name"`
	Machine string    `json:"machine"`
	Text    string    `json:"text"`
	Room    string    `json:"room"`
	At      time.Time `json:"at"`
}

func (s Status) open(now time.Time) bool { return !s.Closed && now.Before(s.Expires) }

func (h *APIHandler) selfID() string {
	if h.records == nil || h.records.Identity() == nil {
		return ""
	}
	return h.records.Identity().ID
}

// qualify writes a member ID as node::session; IDs of agents on other
// machines already carry their node.
func (h *APIHandler) qualify(id string) string {
	if id == "" || strings.Contains(id, "::") || h.selfID() == "" {
		return id
	}
	return h.selfID() + "::" + id
}

// localize turns a qualified ID back into the form this machine uses: a bare
// session ID for its own agents, node::session for the rest.
func (h *APIHandler) localize(id string) string {
	if self := h.selfID(); self != "" {
		return strings.TrimPrefix(id, self+"::")
	}
	return id
}

// machineOf names the machine a qualified member runs on.
func (h *APIHandler) machineOf(ctx context.Context, id string) string {
	node, _, remote := strings.Cut(h.localize(id), "::")
	if !remote {
		host, _ := os.Hostname()
		return host
	}
	var p Peer
	if h.peerDoc(ctx, node, &p) == nil && p.Name != "" {
		return p.Name
	}
	return short(node)
}

func newStatusID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return "s" + hex.EncodeToString(b)
}

func (h *APIHandler) statusList(ctx context.Context) []Status {
	raw, _ := h.store.Documents(ctx, "status")
	out := []Status{}
	for _, b := range raw {
		var s Status
		if json.Unmarshal(b, &s) == nil && s.ID != "" {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At.After(out[j].At) })
	return out
}

func (h *APIHandler) statusGet(ctx context.Context, id string) (Status, bool) {
	var s Status
	return s, h.store.Document(ctx, "status", id, &s) == nil && s.ID != ""
}

// statusSave stores a status, tells the apps, and on its home machine sends
// it to every linked machine.
func (h *APIHandler) statusSave(ctx context.Context, s Status) {
	s.Updated = time.Now().UTC()
	h.statusMu.Lock()
	h.store.PutDocument(ctx, "status", s.ID, s)
	h.statusMu.Unlock()
	h.events.publish("status", s)
	if s.Home == h.selfID() {
		go h.statusPush(context.Background(), s)
	}
}

// statusPush sends a status to every available linked machine.
func (h *APIHandler) statusPush(ctx context.Context, s Status) {
	for _, n := range h.knownNodes(ctx) {
		if n.ID == h.selfID() || !h.nodeAvailable(n.ID) {
			continue
		}
		c, cancel := context.WithTimeout(ctx, 8*time.Second)
		if err := h.nodeRequest(c, n, "POST", "/api/v1/node/status", s, nil); err != nil {
			log.Printf("[MACE] Status %s to %s: %v", s.ID, n.Name, err)
		}
		cancel()
	}
}

// statusPull fetches a machine's statuses, for when it was unreachable while
// they were posted.
func (h *APIHandler) statusPull(ctx context.Context, n Node) {
	var list []Status
	if err := h.nodeRequest(ctx, n, "GET", "/api/v1/node/statuses", nil, &list); err != nil {
		return
	}
	for _, s := range list {
		h.statusReceive(ctx, s)
	}
}

// statusReceive keeps another machine's status if it is newer than ours.
func (h *APIHandler) statusReceive(ctx context.Context, s Status) {
	if s.ID == "" || s.Home == h.selfID() {
		return
	}
	h.statusMu.Lock()
	var old Status
	if h.store.Document(ctx, "status", s.ID, &old) == nil && !s.Updated.After(old.Updated) {
		h.statusMu.Unlock()
		return
	}
	h.store.PutDocument(ctx, "status", s.ID, s)
	h.statusMu.Unlock()
	h.events.publish("status", s)
}

// nodeStatus serves POST /api/v1/node/status: a linked machine's status.
func (h *APIHandler) nodeStatus(w http.ResponseWriter, r *http.Request) {
	n, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var s Status
	if json.Unmarshal(body, &s) != nil || s.Home != n.ID {
		http.Error(w, "A machine can only send its own statuses.", 400)
		return
	}
	h.statusReceive(r.Context(), s)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// nodeStatuses serves GET /api/v1/node/statuses: this machine's own statuses.
func (h *APIHandler) nodeStatuses(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.verifyNodeRequest(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	out := []Status{}
	for _, s := range h.statusList(r.Context()) {
		if s.Home == h.selfID() && time.Since(s.Expires) < statusLife {
			out = append(out, s)
		}
	}
	writeJSON(w, 200, out)
}

type statusAction struct {
	Status string `json:"status"`
	Member string `json:"member"`
	Text   string `json:"text,omitempty"`
	Pick   string `json:"pick,omitempty"`
	Close  bool   `json:"close,omitempty"`
}

// nodeStatusAction serves POST /api/v1/node/status-action: an offer, pick, or
// close made on another machine for a status that lives here.
func (h *APIHandler) nodeStatusAction(w http.ResponseWriter, r *http.Request) {
	n, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var a statusAction
	if json.Unmarshal(body, &a) != nil || !strings.HasPrefix(a.Member, n.ID+"::") && a.Member != "you" {
		http.Error(w, "A machine can only act for its own agents.", 400)
		return
	}
	if err := h.statusApply(r.Context(), a); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// statusAct applies an action here when the status lives here, or sends it
// to the status's home machine.
func (h *APIHandler) statusAct(ctx context.Context, a statusAction) error {
	s, ok := h.statusGet(ctx, a.Status)
	if !ok {
		return fmt.Errorf("There is no status %s.", a.Status)
	}
	if s.Home == h.selfID() {
		return h.statusApply(ctx, a)
	}
	for _, n := range h.knownNodes(ctx) {
		if n.ID == s.Home {
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return h.nodeRequest(c, n, "POST", "/api/v1/node/status-action", a, nil)
		}
	}
	return fmt.Errorf("The machine status %s came from is not linked.", a.Status)
}

// statusApply runs an offer, pick, or close on the status's home machine.
func (h *APIHandler) statusApply(ctx context.Context, a statusAction) error {
	h.statusActMu.Lock()
	defer h.statusActMu.Unlock()
	s, ok := h.statusGet(ctx, a.Status)
	if !ok || s.Home != h.selfID() {
		return fmt.Errorf("Status %s does not live on this machine.", a.Status)
	}
	if !s.open(time.Now()) {
		return fmt.Errorf("Status %s is already closed.", s.ID)
	}
	if a.Pick != "" || a.Close {
		if a.Member != "you" && a.Member != s.For && a.Member != s.Poster {
			return fmt.Errorf("Only the agent who needs it, or whoever posted it, can close status %s.", s.ID)
		}
		h.statusClose(ctx, s, a.Pick)
		return nil
	}
	if a.Member == s.For {
		return fmt.Errorf("That is your own status.")
	}
	for _, o := range s.Offers {
		if o.Member == a.Member {
			return fmt.Errorf("You already offered on status %s; talk in that side chat.", s.ID)
		}
	}
	if len(s.Offers) >= statusOffersLimit {
		return fmt.Errorf("Status %s already has enough offers.", s.ID)
	}
	var group Room
	if h.store.Document(ctx, "room", s.Room, &group) != nil || group.Removed {
		return fmt.Errorf("The group of status %s is gone.", s.ID)
	}
	offerer, requester := h.localize(a.Member), h.localize(s.For)
	side, _, err := h.sideRoom(ctx, &group, requester, offerer)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(a.Text)
	if text == "" {
		text = "I can help with this."
	}
	s.Offers = append(s.Offers, StatusOffer{Member: a.Member, Name: h.agentName(ctx, offerer), Machine: h.machineOf(ctx, a.Member), Text: limited(text, 280), Room: side.ID, At: time.Now().UTC()})
	h.statusSave(ctx, s)
	m := Message{ID: newID(), Room: side.ID, Sender: offerer, Text: fmt.Sprintf("Offer for your status %s (%s):\n%s", s.ID, limited(s.Text, 200), text), Flair: "offer", At: time.Now().UTC(), Delivery: map[string]string{}, Mentions: []string{requester}, Hop: 1}
	if _, err := h.postMessage(ctx, &m); err != nil {
		return err
	}
	h.commandNote(s.Room, fmt.Sprintf("%s offered help on status %s. You can read their side chat.", h.memberName(ctx, offerer), s.ID), false)
	return h.dispatch(ctx, side.ID, m)
}

// statusClose closes a status, thanks the offers not picked, and notes the
// result in the group.
func (h *APIHandler) statusClose(ctx context.Context, s Status, pick string) {
	s.Closed, s.Picked = true, pick
	h.statusSave(ctx, s)
	picked := ""
	for _, o := range s.Offers {
		if o.Member == pick {
			picked = o.Name
			continue
		}
		h.commandNote(o.Room, fmt.Sprintf("Status %s is sorted, thanks for offering.", s.ID), false)
	}
	if picked != "" {
		// Hand the work back: the requester carries on in its own group with
		// what it got, so the owner sees the result where they asked.
		requester := h.localize(s.For)
		var group Room
		if h.store.Document(ctx, "room", s.Room, &group) == nil && !group.Removed && slices.Contains(group.Members, requester) {
			handle := h.roomHandles(ctx, group.Members)[requester]
			m := Message{ID: newID(), Room: s.Room, Sender: "mace", Kind: "system", Wake: true, Mentions: []string{requester}, At: time.Now().UTC(), Delivery: map[string]string{},
				Text: fmt.Sprintf("Status %s is sorted: %s went with %s. @%s, carry on with the task here using what they gave you in the side chat.", s.ID, s.ForName, picked, handle)}
			if _, err := h.postMessage(ctx, &m); err == nil {
				h.dispatch(ctx, s.Room, m)
			}
		} else {
			h.commandNote(s.Room, fmt.Sprintf("Status %s is sorted: %s went with %s.", s.ID, s.ForName, picked), false)
		}
	} else {
		h.commandNote(s.Room, fmt.Sprintf("Status %s is closed.", s.ID), false)
	}
}

// statusTags handles status tags in an agent's reply in a conversation on
// this machine. It returns the text without them, a flair for a new status,
// and whether the reply was an offer, which goes to a side chat instead.
func (h *APIHandler) statusTags(ctx context.Context, room string, group *Room, p Policy, members []string, member, text string) (string, string, bool) {
	flair := ""
	if m := offerPattern.FindStringSubmatch(text); m != nil {
		rest := strings.TrimSpace(strings.Replace(text, m[0], "", 1))
		if err := h.statusAct(ctx, statusAction{Status: strings.ToLower(m[1]), Member: h.qualify(member), Text: rest}); err != nil {
			h.commandNote(room, h.memberName(ctx, member)+" could not offer: "+err.Error(), false)
		}
		return "", "", true
	}
	h.untaggedOffer(ctx, room, member, text)
	if m := statusPickPattern.FindStringSubmatch(text); m != nil {
		text = strings.TrimSpace(strings.Replace(text, m[0], "", 1))
		h.statusPick(ctx, room, member, m[1])
	} else if m := statusDonePattern.FindString(text); m != "" {
		text = strings.TrimSpace(strings.Replace(text, m, "", 1))
		for _, s := range h.statusList(ctx) {
			if s.open(time.Now()) && s.For == h.qualify(member) {
				if err := h.statusAct(ctx, statusAction{Status: s.ID, Member: s.For, Close: true}); err != nil {
					h.commandNote(room, err.Error(), false)
				}
			}
		}
	}
	m := statusPostPattern.FindStringSubmatchIndex(text)
	if m == nil || group == nil {
		return text, flair, false
	}
	need := strings.TrimSpace(text[m[2]:m[3]])
	text = strings.TrimSpace(text[:m[0]] + text[m[1]:])
	if p.Mode == "team" && p.Lead != "" && member != p.Lead {
		h.commandNote(room, h.memberName(ctx, member)+" tried to post a status; in a team only the lead posts them. Ask @lead.", false)
		return text, flair, false
	}
	forID := member
	if f := statusForPattern.FindStringSubmatchIndex(need); f != nil {
		handle := strings.ToLower(need[f[2]:f[3]])
		for id, hd := range h.roomHandles(ctx, members) {
			if strings.ToLower(hd) == handle || strings.SplitN(strings.ToLower(hd), "@", 2)[0] == handle {
				forID = id
			}
		}
		need = strings.TrimSpace(need[:f[0]])
	}
	if err := h.statusPost(ctx, group, member, forID, need); err != nil {
		h.commandNote(room, "Status not posted: "+err.Error(), false)
		return text, flair, false
	}
	if text == "" {
		text = need
	}
	return text, "status", false
}

// statusPick closes the requester's open status with the offer whose agent
// the handle names.
func (h *APIHandler) statusPick(ctx context.Context, room, member, handle string) {
	handle = strings.ToLower(handle)
	base := strings.SplitN(handle, "@", 2)[0]
	for _, s := range h.statusList(ctx) {
		if !s.open(time.Now()) || s.For != h.qualify(member) && s.Poster != h.qualify(member) {
			continue
		}
		for _, o := range s.Offers {
			hd := strings.ToLower(h.memberHandle(ctx, h.localize(o.Member)))
			if hd == handle || hd == base {
				if err := h.statusAct(ctx, statusAction{Status: s.ID, Member: h.qualify(member), Pick: o.Member}); err != nil {
					h.commandNote(room, err.Error(), false)
				}
				return
			}
		}
	}
	// A pick repeated after the status closed is not worth a note.
	for _, s := range h.statusList(ctx) {
		if s.open(time.Now()) && (s.For == h.qualify(member) || s.Poster == h.qualify(member)) {
			h.commandNote(room, "No offer from @"+handle+" on your status "+s.ID+".", false)
			return
		}
	}
}

// statusPost records a new status for an agent, replacing its older one.
func (h *APIHandler) statusPost(ctx context.Context, group *Room, poster, forID, need string) error {
	if need == "" {
		return fmt.Errorf("say what is needed")
	}
	now := time.Now().UTC()
	inGroup := 0
	for _, s := range h.statusList(ctx) {
		if !s.open(now) {
			continue
		}
		if s.For == h.qualify(forID) && s.Home == h.selfID() {
			s.Closed = true
			h.statusSave(ctx, s)
			continue
		}
		if s.Room == group.ID {
			inGroup++
		}
	}
	if inGroup >= statusPerGroup {
		return fmt.Errorf("this group already has %d open statuses", statusPerGroup)
	}
	s := Status{
		ID: newStatusID(), Home: h.selfID(), Text: limited(need, 280),
		For: h.qualify(forID), ForName: h.agentName(ctx, forID), Machine: h.machineOf(ctx, h.qualify(forID)),
		Poster: h.qualify(poster), Room: group.ID, RoomName: group.Name,
		At: now, Expires: now.Add(statusLife),
	}
	h.statusSave(ctx, s)
	return nil
}

// statusBoard is the awareness section an agent reads each turn: open
// statuses of others, and the offers on its own.
func (h *APIHandler) statusBoard(ctx context.Context, s Session) string {
	now := time.Now()
	self := h.qualify(s.ID)
	var b strings.Builder
	others := 0
	for _, st := range h.statusList(ctx) {
		if !st.open(now) {
			continue
		}
		if st.For == self {
			fmt.Fprintf(&b, "YOUR STATUS %s: %s.", st.ID, st.Text)
			if len(st.Offers) == 0 {
				b.WriteString(" No offers yet.\n")
				continue
			}
			b.WriteString(" Offers, each in its own side chat:\n")
			for _, o := range st.Offers {
				fmt.Fprintf(&b, "- @%s (%s on %s): %s\n", h.memberHandle(ctx, h.localize(o.Member)), o.Name, o.Machine, limited(o.Text, 160))
			}
			b.WriteString("Talk with any of them in their side chats, then pick one with [status pick @handle], or close it with [status done].\n")
			continue
		}
		if others == statusBoardLimit || slices.ContainsFunc(st.Offers, func(o StatusOffer) bool { return o.Member == self }) {
			continue
		}
		if others == 0 {
			b.WriteString("STATUS BOARD (requests from other agents; offer only if you really have it):\n")
		}
		others++
		fmt.Fprintf(&b, "- %s: %s on %s needs %s (group %q, %s ago, %d offers)\n", st.ID, st.ForName, st.Machine, st.Text, limited(st.RoomName, 60), ago(now.Sub(st.At)), len(st.Offers))
	}
	if others > 0 {
		b.WriteString("The agent who asked is usually not in your conversations and cannot see them: only a message that starts with [offer <id>] reaches it, as a side chat with it. Answering anywhere else is lost. Offer only what you can actually provide; if you cannot help, say nothing about it.\n")
	}
	return b.String()
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%d m", int(d.Minutes()))
	default:
		return fmt.Sprintf("%d h", int(d.Hours()))
	}
}

// statuses serves GET /api/v1/statuses to the owner's apps: open statuses
// and those closed within the last day.
func (h *APIHandler) statuses(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	out := []Status{}
	for _, s := range h.statusList(r.Context()) {
		if s.open(now) || now.Sub(s.Updated) < statusLife {
			out = append(out, s)
		}
	}
	writeJSON(w, 200, out)
}

// closeStatus serves POST /api/v1/statuses/{id}/close for the owner.
func (h *APIHandler) closeStatus(w http.ResponseWriter, r *http.Request) {
	if err := h.statusAct(r.Context(), statusAction{Status: chi.URLParam(r, "id"), Member: "you", Close: true}); err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// untaggedOffer is the safety net for an agent that answers a status in its
// own conversation instead of with [offer <id>]: if the reply names an open
// status, sounds like an offer and not a refusal, and the agent is not in the
// requester's group (where the requester would see it anyway), the reply is
// also sent as an offer, and the conversation is told so.
func (h *APIHandler) untaggedOffer(ctx context.Context, room, member, text string) {
	if offerPattern.MatchString(text) || !offerWords.MatchString(text) || refusalWords.MatchString(text) {
		return
	}
	self := h.qualify(member)
	for _, id := range statusIDPattern.FindAllString(text, 3) {
		st, ok := h.statusGet(ctx, id)
		if !ok || !st.open(time.Now()) || st.For == self || st.Poster == self {
			continue
		}
		var group Room
		if st.Home == h.selfID() && h.store.Document(ctx, "room", st.Room, &group) == nil && slices.Contains(group.Members, member) {
			continue
		}
		if err := h.statusAct(ctx, statusAction{Status: st.ID, Member: self, Text: text}); err == nil {
			h.commandNote(room, fmt.Sprintf("Mace sent %s's message to %s as an offer on status %s, since they cannot see this chat.", h.memberName(ctx, member), st.ForName, st.ID), false)
		}
		return
	}
}

// agentName is an agent's own name, without the "machine / " prefix the hub
// keeps on agents of other machines; the machine is shown on its own.
func (h *APIHandler) agentName(ctx context.Context, id string) string {
	name := h.memberName(ctx, id)
	if _, after, ok := strings.Cut(name, " / "); ok && strings.Contains(id, "::") {
		return after
	}
	return name
}
