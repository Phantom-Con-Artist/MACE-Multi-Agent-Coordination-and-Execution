package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mace/internal/storage"
)

func limited(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// agentInstructions is sent once, when a native session starts. Resumed
// sessions already hold it; the awareness block keeps them current.
func agentInstructions(s Session) string {
	return fmt.Sprintf(`You are %s, an AI coworker connected through MACE (Multi Agent Coordination and Execution).
Mace is the owner's private browser workspace for native CLI agents, host files, apps, and collaboration. The owner can reach this machine from a phone or another computer. Your replies appear as your own messages in Mace. You are a real native CLI agent with your existing tools, login, and permissions, not a simulated chat character.
You are one coworker with one memory: your private chat with the owner and every group you join share this same session. Mace tells you which conversation each turn is for.
Speak in your own voice; never invent messages or completed work for other participants. Do not introduce yourself or explain Mace on every turn. Do not produce repetitive ready/status reports or robotic acknowledgments.
Roles are unassigned by default. In a team, volunteer for useful responsibilities naturally, let specialization emerge from the work, and coordinate with the other agents. Be candid about disagreement, uncertainty, and what you actually checked. Do not pretend to be human.
Work on the owner's actual task with your available native tools when needed. Agents on the same host can use the same filesystem paths without copying data through Mace. Cross-host attachments are explicitly transferred and their destination paths are supplied. Do not claim a transfer or tool action happened unless it did.
`, limited(s.Name, 120))
}

const conventions = `MACE CONVENTIONS:
- Write @handle to hand a coworker the next turn. Only mentioned coworkers respond to your message.
- If you have nothing useful to add, reply with exactly [PASS]. It is not posted.
- You may start a message with a flair that says what it is, such as [question], [plan], [blocker], or [done].
- When a request is finished, start your message with [done] and one line of proof.
`

// chatStyle is how group messages should read. Casual groups write like a
// messenger chat; professional ones stay short without slang.
func chatStyle(tone string) string {
	if tone == "professional" {
		return "STYLE (professional group): one or two short, plain sentences per message. No slang, no headings, no bullet walls, no restating what others said. Go long only when the owner asks for detail or you are delivering findings, a plan, code, or a decision.\n"
	}
	return "STYLE: write like a group chat. One or two short lines per message; casual shorthand is fine (ik, ngl, tbh, imo, lol, lmao). No headings, no bullet walls, no restating what others said. Go long only when the owner asks for detail or you are delivering findings, a plan, code, or a decision.\n"
}

// longReply is the length after which a routine reply earns a reminder.
const longReply = 500

// awareness tells an agent who it is and where it is in Mace right now. It is
// built on the agent's own host, from that host's conversations.
func (h *APIHandler) awareness(ctx context.Context, s Session) string {
	var b strings.Builder
	host, _ := os.Hostname()
	model := s.Model
	if model == "" {
		model = "the CLI default"
	}
	fmt.Fprintf(&b, "MACE AWARENESS: You are %s (@%s) on host %s. Model: %s. Working folder: %s.\n", limited(s.Name, 120), s.Handle, host, model, s.Directory)
	if s.Role != "" {
		fmt.Fprintf(&b, "Your role, set by the owner: %s. A group may assign you a different role there.\n", limited(s.Role, 600))
	}
	b.WriteString("Your conversations:\n")
	fmt.Fprintf(&b, "- Private chat with the owner.%s\n", h.latest(ctx, s.ID))
	raw, _ := h.store.Documents(ctx, "room")
	listed := 0
	for _, r := range raw {
		var g Room
		if json.Unmarshal(r, &g) != nil || g.Removed || !containsID(g.Members, s.ID) {
			continue
		}
		if listed++; listed > 12 {
			b.WriteString("- More groups are not listed.\n")
			break
		}
		handles := h.roomHandles(ctx, g.Members)
		names := []string{}
		for _, m := range g.Members {
			names = append(names, "@"+handles[m])
		}
		mode := h.policyFor(ctx, g.ID, g.Members).Mode
		fmt.Fprintf(&b, "- Group %q with %s (%s mode).%s\n", limited(g.Name, 80), strings.Join(names, ", "), mode, h.latest(ctx, g.ID))
	}
	return b.String() + conventions
}

// profileRole is the role an agent carries into every conversation.
func (h *APIHandler) profileRole(ctx context.Context, id string) string {
	var s Session
	if h.store.Document(ctx, "session", id, &s) == nil || h.store.Document(ctx, "remote-session", id, &s) == nil {
		return strings.TrimSpace(s.Role)
	}
	return ""
}

func containsID(list []string, id string) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// latest summarizes the newest message of a conversation in one line.
func (h *APIHandler) latest(ctx context.Context, room string) string {
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 8)
	var m Message
	for i := len(rows) - 1; i >= 0; i-- {
		if json.Unmarshal(rows[i].Data, &m) == nil && m.Kind != "note" {
			break
		}
		m = Message{}
	}
	if m.ID == "" {
		return ""
	}
	return fmt.Sprintf(" Latest: %s: %q", h.memberName(ctx, m.Sender), limited(strings.Join(strings.Fields(m.Text), " "), 140))
}

func (h *APIHandler) memberName(ctx context.Context, id string) string {
	if id == "you" {
		return "Owner"
	}
	if id == "mace" {
		return "Mace"
	}
	var s Session
	if h.store.Document(ctx, "session", id, &s) == nil {
		return limited(s.Name, 120)
	}
	if h.store.Document(ctx, "remote-session", id, &s) == nil {
		return limited(s.Name, 120)
	}
	if p, e := h.supervisor.GetAgent(id); e == nil && p != nil {
		return limited(p.Name, 120)
	}
	return limited(id, 120)
}

// turnPrompt carries both forms of a turn. Full suits a fresh native session;
// Delta holds only what is new since the agent's last turn in this room. The
// host that runs the agent picks one, since only it knows the session state.
type turnPrompt struct {
	Full  string `json:"full"`
	Delta string `json:"delta,omitempty"`
}

// conversationHeader describes the room and the agent's place in it.
func (h *APIHandler) conversationHeader(ctx context.Context, room, member string, members []string, group *Room, p Policy) string {
	var header strings.Builder
	handles := h.roomHandles(ctx, members)
	if group == nil {
		header.WriteString("THIS TURN IS FOR YOUR PRIVATE CHAT with the owner. Answer the owner's latest message naturally and directly. Keep routine replies short; go long only when the work needs it. Ask a brief clarifying question if the task needs it.\n")
		if p.Mode == "goal" {
			fmt.Fprintf(&header, "OWNER'S GOAL: %s\n", p.Goal)
		}
		return header.String()
	}
	fmt.Fprintf(&header, "THIS TURN IS FOR MACE GROUP: %s\nYou are speaking as %s (@%s).\n", limited(group.Name, 200), h.memberName(ctx, member), handles[member])
	if request := h.latestOwnerRequest(ctx, room); request != "" {
		fmt.Fprintf(&header, "OWNER'S LATEST REQUEST: %q\n", limited(request, 600))
	}
	if p.Mode == "goal" {
		fmt.Fprintf(&header, "OWNER'S GOAL: %s\nWork toward this goal together. Only when it is achieved, start your message with [done] and concrete evidence.\n", limited(p.Goal, 1000))
	}
	header.WriteString("TEAM:\n")
	for index, other := range members {
		if index >= 40 {
			header.WriteString("Additional members are present.\n")
			break
		}
		assigned := strings.TrimSpace(group.Roles[other])
		if assigned == "" {
			assigned = h.profileRole(ctx, other)
		}
		chosen := strings.TrimSpace(group.Claimed[other])
		var role string
		switch {
		case assigned != "" && chosen != "":
			role = fmt.Sprintf("Owner-assigned role: %s. Also taking on (self-chosen): %s", limited(assigned, 600), limited(chosen, 200))
		case assigned != "":
			role = "Owner-assigned role: " + limited(assigned, 600)
		case chosen != "":
			role = "Self-chosen role: " + limited(chosen, 200)
		default:
			role = "No role yet"
		}
		fmt.Fprintf(&header, "- %s (@%s). %s\n", h.memberName(ctx, other), handles[other], role)
	}
	header.WriteString("HOW THIS TEAM WORKS: you are coworkers on the owner's request, all able to work at once. Split the work by what each of you is best placed to do, like a real team. Roles are not fixed: when a piece of work has no owner, claim it by starting a message with [role: what you take on], and claim again when the task shifts. Owner-assigned roles come first. Don't redo work someone else claimed; hand off with @handle; never speak for a coworker. If the owner's message is just chat, answer in a line and stop.\n")
	header.WriteString(chatStyle(p.Tone))
	fmt.Fprintf(&header, "FLAIRS in this group: %s. You may add a new flair if it helps the request (up to %d custom).\n", "["+strings.Join(groupFlairs(group), "] [")+"]", maxCustomFlairs)
	if n := h.lastReplyLength(ctx, room, member); n > longReply {
		fmt.Fprintf(&header, "Your last message here was %d characters. Keep this one to a line or two unless it is a deliverable.\n", n)
	}
	switch p.Mode {
	case "continuous":
		header.WriteString("UNTIL-DONE MODE: keep working with your coworkers on the owner's latest request without waiting for another owner message. When the request is done, start your message with [done] and one line of proof. If you are waiting on someone or have nothing to add, reply [PASS]. No small talk and no new tasks of your own when there is nothing to do.\n")
	case "reply":
		fmt.Fprintf(&header, "This conversation has a total reply budget of %d. Use your turn to contribute, not to emit a status report.\n", p.Limit)
	}
	return header.String()
}

// latestOwnerRequest is the owner's newest message in a conversation.
func (h *APIHandler) latestOwnerRequest(ctx context.Context, room string) string {
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 40)
	for i := len(rows) - 1; i >= 0; i-- {
		var m Message
		if json.Unmarshal(rows[i].Data, &m) == nil && m.Sender == "you" {
			return strings.Join(strings.Fields(m.Text), " ")
		}
	}
	return ""
}

// lastReplyLength is the length of a member's newest routine message in a
// conversation; deliverables (plans, decisions, done reports) count as zero.
func (h *APIHandler) lastReplyLength(ctx context.Context, room, member string) int {
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 40)
	for i := len(rows) - 1; i >= 0; i-- {
		var m Message
		if json.Unmarshal(rows[i].Data, &m) != nil || m.Sender != member {
			continue
		}
		if m.Flair == "plan" || m.Flair == "decision" || m.Flair == "done" {
			return 0
		}
		return len(m.Text)
	}
	return 0
}

func (h *APIHandler) transcript(ctx context.Context, rows []json.RawMessage) string {
	var t strings.Builder
	for _, b := range rows {
		var m Message
		if json.Unmarshal(b, &m) != nil || m.Kind == "note" {
			continue
		}
		fmt.Fprintf(&t, "%s: %s\n", h.memberName(ctx, m.Sender), m.Text)
		if m.File != "" {
			fmt.Fprintf(&t, "Shared file on the originating host: %s\n", filepath.Join(h.fileRoot, m.File))
		}
	}
	return t.String()
}

// buildPrompts prepares a turn and returns the sequence of the newest message
// it includes, which becomes the agent's read cursor once the turn succeeds.
//
// fresh reports whether anyone else wrote since the agent's cursor; a first
// turn is always fresh.
func (h *APIHandler) buildPrompts(ctx context.Context, req *turnRequest, members []string, group *Room, p Policy) (turnPrompt, int64, bool) {
	header := h.conversationHeader(ctx, req.Room, req.Member, members, group, p)
	recent, _, _ := h.store.Messages(ctx, req.Room, 0, 0, 40)
	raw := func(rows []storage.StoredMessage) []json.RawMessage {
		out := []json.RawMessage{}
		for _, r := range rows {
			out = append(out, r.Data)
		}
		return out
	}
	var last int64
	if len(recent) > 0 {
		last = recent[len(recent)-1].Seq
	}
	full := header + "\nSHARED CONVERSATION (oldest to newest):\n" + truncate(h.transcript(ctx, raw(recent)), 72000) + "\nIt is your turn. Contribute your own next message.\n"
	since := h.cursor(ctx, req.Member, req.Room)
	if since == 0 {
		return turnPrompt{Full: full}, last, true
	}
	newer, more, _ := h.store.Messages(ctx, req.Room, 0, since, 40)
	note := ""
	if more {
		note = "(More messages arrived than fit here; the newest are below. Ask if you need the rest.)\n"
	}
	delta := header + "\nNEW IN THIS CONVERSATION SINCE YOUR LAST TURN (oldest to newest):\n" + note + truncate(h.transcript(ctx, raw(newer)), 72000) + "\nIt is your turn. Contribute your own next message.\n"
	fresh := false
	for _, row := range newer {
		var m Message
		if json.Unmarshal(row.Data, &m) == nil && m.Sender != req.Member && m.Kind != "note" {
			fresh = true
		}
	}
	if len(newer) > 0 {
		last = max(last, newer[len(newer)-1].Seq)
	}
	return turnPrompt{Full: full, Delta: delta}, last, fresh
}

func (h *APIHandler) available(ctx context.Context, id string) bool {
	var s Session
	if strings.Contains(id, "::") {
		// An agent the peer has never reported is not assumed to exist.
		if h.store.Document(ctx, "remote-session", id, &s) == nil {
			return s.Status != "removed" && s.Status != "stopped" && s.Status != "terminated"
		}
		return false
	}
	if h.store.Document(ctx, "session", id, &s) == nil {
		return s.Status == "ready" || s.Status == "replying"
	}
	p, e := h.supervisor.GetAgent(id)
	return e == nil && p != nil && !p.Status.IsTerminal()
}
