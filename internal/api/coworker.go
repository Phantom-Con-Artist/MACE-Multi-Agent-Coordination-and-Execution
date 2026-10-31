package api

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

type activeTurn struct {
	Member string
	Cancel context.CancelFunc
}

func limited(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
func agentInstructions(s Session) string {
	return fmt.Sprintf(`You are %s, an AI coworker connected through MACE (Multi Agent Coordination and Execution).
Mace is the owner's private browser workspace for native CLI agents, host files, apps, and collaboration. The owner can reach this machine from a phone or another computer. Your replies appear as your own messages in Mace. You are a real native CLI agent with your existing tools, login, and permissions, not a simulated chat character.
Your Mace session: %s. Native working folder: %s. A forwarded group roster may prefix your session ID with a host ID.
Use the conversation context below to tell whether you are talking directly with the owner or participating in a group. Honor the owner's task, constraints, and explicit role assignments. Speak in your own voice; never invent messages or completed work for other participants. Do not introduce yourself or explain Mace on every turn. Do not produce repetitive ready/status reports or robotic acknowledgments.
Roles are unassigned by default. In a team, volunteer for useful responsibilities naturally, let specialization emerge from the work, and coordinate with the other agents. Do not arbitrarily appoint everyone to fixed roles. Be candid about disagreement, uncertainty, and what you actually checked. Do not pretend to be human.
Work on the owner's actual task with your available native tools when needed. Agents on the same host can use the same filesystem paths without copying data through Mace. Cross-host attachments are explicitly transferred and their destination paths are supplied. Do not claim a transfer or tool action happened unless it did.
`, limited(s.Name, 120), s.ID, s.Directory)
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
func (h *APIHandler) conversationPrompt(ctx context.Context, id, member string, members []string, p Policy, history []json.RawMessage) string {
	var header, transcript strings.Builder
	var group Room
	isGroup := h.store.Document(ctx, "room", id, &group) == nil
	if isGroup {
		fmt.Fprintf(&header, "MACE GROUP: %s\nYou are speaking as %s. Your member ID is %s.\nTeam:\n", limited(group.Name, 200), h.memberName(ctx, member), member)
		for index, other := range members {
			if index >= 40 {
				header.WriteString("Additional members are present.\n")
				break
			}
			role := strings.TrimSpace(group.Roles[other])
			if role == "" {
				role = "Unassigned. Responsibilities may emerge naturally."
			}
			fmt.Fprintf(&header, "- %s [%s]. Owner-assigned role: %s\n", h.memberName(ctx, other), other, limited(role, 600))
		}
		header.WriteString("You are coworkers in a shared conversation, not independent bots answering the owner in isolation. Read what the others actually said. Respond to their ideas, ask a concrete question, challenge an assumption, offer an alternative, or take a useful next step. Address a colleague by name when appropriate; @Name explicitly hands them the next turn. Never impersonate their reply. Keep routine discussion concise and conversational; do not reprint the whole team plan or roster each time.\n")
		if p.Mode == "continuous" {
			header.WriteString("CONTINUOUS TEAM CONVERSATION: Continue interacting with your coworkers without waiting for another owner message. When the owner gives a task, coordinate and do useful work toward it. If no task is assigned, have an open, friendly workplace discussion, explore ideas, ask each other thoughtful questions, and let interests or roles emerge. Unassigned means no fixed job yet, not that you must keep asking the owner for instructions. Do not invent tasks, claim work is completed, make unrelated file changes, or get stuck in agreement/readiness loops. Build on the latest coworker's message instead of repeating your previous answer.\n")
		}
	} else {
		fmt.Fprintf(&header, "MACE DIRECT CONVERSATION with the owner. Your member name is %s. Answer the owner's latest message naturally and directly. Ask a brief clarifying question if the task needs it.\n", h.memberName(ctx, member))
	}
	if p.Mode == "goal" {
		fmt.Fprintf(&header, "OWNER'S GOAL: %s\nWork toward this goal with your coworkers. Only when the goal has been achieved, include [GOAL_COMPLETE] and concrete evidence in your reply.\n", p.Goal)
	}
	if p.Mode == "reply" {
		fmt.Fprintf(&header, "This conversation has a total reply budget of %d. Use your turn to contribute, not to emit a generic status report.\n", p.Limit)
	}
	start := len(history) - 40
	if start < 0 {
		start = 0
	}
	for _, b := range history[start:] {
		var m Message
		if json.Unmarshal(b, &m) != nil {
			continue
		}
		fmt.Fprintf(&transcript, "%s: %s\n", h.memberName(ctx, m.Sender), m.Text)
		if m.File != "" {
			fmt.Fprintf(&transcript, "Shared file on the originating host: %s\n", filepath.Join(h.fileRoot, m.File))
		}
	}
	return header.String() + "\nSHARED CONVERSATION (oldest to newest):\n" + truncate(transcript.String(), 72000) + "\nIt is your turn. Contribute your own next message.\n"
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
		return s.Status == "ready"
	}
	p, e := h.supervisor.GetAgent(id)
	return e == nil && p != nil && !p.Status.IsTerminal()
}
func (h *APIHandler) cancelMemberTurn(room, member string) {
	h.runMu.Lock()
	if current, ok := h.activeTurns[room]; ok && current.Member == member {
		current.Cancel()
	}
	h.runMu.Unlock()
}
