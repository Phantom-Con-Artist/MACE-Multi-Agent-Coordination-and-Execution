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
- In goal mode, include [GOAL_COMPLETE] with concrete evidence only when the goal is achieved.
`

// awareness tells an agent who it is and where it is in Mace right now. It is
// built on the agent's own host, from that host's conversations.
func (h *APIHandler) awareness(ctx context.Context, s Session) string {
	var b strings.Builder
	host, _ := os.Hostname()
	model := s.Model
	if model == "" {
		model = "the CLI default"
	}
	fmt.Fprintf(&b, "MACE AWARENESS: You are %s (@%s) on host %s. Model: %s. Working folder: %s.\nYour conversations:\n", limited(s.Name, 120), s.Handle, host, model, s.Directory)
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
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 1)
	if len(rows) == 0 {
		return ""
	}
	var m Message
	json.Unmarshal(rows[0].Data, &m)
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
	if group != nil {
		fmt.Fprintf(&header, "THIS TURN IS FOR MACE GROUP: %s\nYou are speaking as %s (@%s).\nTeam:\n", limited(group.Name, 200), h.memberName(ctx, member), handles[member])
		for index, other := range members {
			if index >= 40 {
				header.WriteString("Additional members are present.\n")
				break
			}
			role := strings.TrimSpace(group.Roles[other])
			if role == "" {
				role = "Unassigned. Responsibilities may emerge naturally."
			}
			fmt.Fprintf(&header, "- %s (@%s). Owner-assigned role: %s\n", h.memberName(ctx, other), handles[other], limited(role, 600))
		}
		header.WriteString("You are coworkers in a shared conversation, and everyone may be working at the same time. Read what the others actually said. Respond to their ideas, ask a concrete question, challenge an assumption, offer an alternative, or take a useful next step. Mention a colleague with @handle to hand them a turn; never impersonate their reply. Keep routine discussion concise and conversational; do not reprint the whole team plan or roster each time.\n")
		if p.Mode == "continuous" {
			header.WriteString("CONTINUOUS TEAM CONVERSATION: Continue interacting with your coworkers without waiting for another owner message. When the owner gives a task, coordinate and do useful work toward it. If no task is assigned, have an open, friendly workplace discussion, explore ideas, ask each other thoughtful questions, and let interests or roles emerge. Do not invent tasks, claim work is completed, make unrelated file changes, or get stuck in agreement/readiness loops. Build on the latest coworker's message instead of repeating your previous answer.\n")
		}
	} else {
		fmt.Fprintf(&header, "THIS TURN IS FOR YOUR PRIVATE CHAT with the owner. Answer the owner's latest message naturally and directly. Ask a brief clarifying question if the task needs it.\n")
	}
	if p.Mode == "goal" {
		fmt.Fprintf(&header, "OWNER'S GOAL: %s\nWork toward this goal with your coworkers. Only when the goal has been achieved, include [GOAL_COMPLETE] and concrete evidence in your reply.\n", p.Goal)
	}
	if p.Mode == "reply" && group != nil {
		fmt.Fprintf(&header, "This conversation has a total reply budget of %d. Use your turn to contribute, not to emit a generic status report.\n", p.Limit)
	}
	return header.String()
}

func (h *APIHandler) transcript(ctx context.Context, rows []json.RawMessage) string {
	var t strings.Builder
	for _, b := range rows {
		var m Message
		if json.Unmarshal(b, &m) != nil {
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
		if json.Unmarshal(row.Data, &m) == nil && m.Sender != req.Member {
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
