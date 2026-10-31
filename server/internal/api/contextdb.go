package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
)

// Turns carry only what is new; agents fetch the rest on demand with
// mace ctx, which reads this host's conversation store. Each agent gets a
// read-only token for its own session, valid until the daemon restarts, and
// can only read conversations it belongs to.
//
// Following ToolCaching (arXiv 2601.15335), fetches are read-only and keyed
// by their parameters: a repeated fetch whose result has not changed since
// the same native session last saw it returns a one-line note instead of the
// same text, so an agent never pays twice for context it already holds.

// maxContextReply caps one fetch, so a broad search cannot flood a turn.
const maxContextReply = 6000

type contextService struct {
	key  []byte
	url  string
	seen sync.Map // session|native|view|room|arg -> result hash
}

func newContextService() *contextService {
	key := make([]byte, 32)
	rand.Read(key)
	return &contextService{key: key}
}

func (c *contextService) token(session string) string {
	m := hmac.New(sha256.New, c.key)
	m.Write([]byte("mace-ctx/1\n" + session))
	return hex.EncodeToString(m.Sum(nil))
}

// contextEnv is what a local agent's process needs to run mace ctx.
func (h *APIHandler) contextEnv(session, room string) map[string]string {
	if h.ctx == nil || h.ctx.url == "" {
		return nil
	}
	return map[string]string{
		"MACE_CTX_URL":   h.ctx.url,
		"MACE_CTX_AGENT": session,
		"MACE_CTX_TOKEN": h.ctx.token(session),
		"MACE_CTX_CHAT":  room,
	}
}

// contextCommand is how agents invoke mace ctx, or "" when the mace command
// cannot be found on this host.
func contextCommand() string {
	if p, err := exec.LookPath("mace"); err == nil {
		return p
	}
	if self, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(self), "mace"); p != self {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

// contextLine tells an agent how to fetch more than its turn carries.
func (h *APIHandler) contextLine(member string) string {
	cmd := contextCommand()
	if strings.Contains(member, "::") || cmd == "" || h.ctx == nil || h.ctx.url == "" {
		return ""
	}
	if p, err := exec.LookPath("mace"); err == nil && p == cmd {
		cmd = "mace"
	} else {
		cmd = strconv.Quote(cmd)
	}
	return "CONTEXT ON DEMAND: your turn shows only what is new. Fetch more only when you need it, with " + cmd + " ctx team | plan | recent [n] | thread @handle | search <words>. A repeated fetch says when nothing changed.\n"
}

var contextViews = []string{"team", "plan", "recent", "thread", "search"}

func (h *APIHandler) contextFetch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agent := r.Header.Get("X-Mace-Agent")
	given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if h.ctx == nil || agent == "" || subtle.ConstantTimeCompare([]byte(given), []byte(h.ctx.token(agent))) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	room, view, arg := chi.URLParam(r, "room"), chi.URLParam(r, "view"), strings.TrimSpace(r.URL.Query().Get("q"))
	members, group, err := h.conversation(ctx, room)
	if err != nil || (group != nil && !containsID(members, agent)) || (group == nil && room != agent) {
		http.Error(w, "you are not in that conversation", http.StatusForbidden)
		return
	}
	var out string
	switch view {
	case "team":
		out = h.contextTeam(ctx, room, agent, members, group)
	case "plan":
		out = h.contextPlan(ctx, room, agent)
	case "recent":
		n, _ := strconv.Atoi(arg)
		out = h.contextRecent(ctx, room, agent, n)
	case "thread":
		out = h.contextThread(ctx, room, agent, members, strings.TrimPrefix(arg, "@"))
	case "search":
		out = h.contextSearch(ctx, room, agent, arg)
	default:
		http.Error(w, "unknown view; use one of: "+strings.Join(contextViews, ", "), http.StatusNotFound)
		return
	}
	if out == "" {
		out = "Nothing found.\n"
	}
	out = truncate(out, maxContextReply)
	var s Session
	h.store.Document(ctx, "session", agent, &s)
	key := strings.Join([]string{agent, s.NativeSession, view, room, arg}, "|")
	sum := sha256.Sum256([]byte(out))
	digest := hex.EncodeToString(sum[:8])
	if prev, ok := h.ctx.seen.Load(key); ok && prev == digest {
		out = "Unchanged since you last fetched this; use what you already have.\n"
	} else {
		h.ctx.seen.Store(key, digest)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(out))
}

func (h *APIHandler) contextTeam(ctx context.Context, room, reader string, members []string, group *Room) string {
	if group == nil {
		return "This is your private chat with the owner; there is no team.\n"
	}
	return h.conversationHeader(ctx, room, reader, members, group, h.policyFor(ctx, room, members))
}

// contextPlan collects the decisions a coworker needs to stay aligned: the
// owner's requests, plans, decisions, role claims, and redirects.
func (h *APIHandler) contextPlan(ctx context.Context, room, reader string) string {
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 1000)
	keep := []json.RawMessage{}
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Data, &m) != nil {
			continue
		}
		switch {
		case m.Sender == "you", m.Role != "":
		case m.Flair == "plan", m.Flair == "decision", m.Flair == "redirect", m.Flair == "done", m.Flair == "blocker":
		default:
			continue
		}
		keep = append(keep, row.Data)
	}
	if len(keep) > 30 {
		keep = keep[len(keep)-30:]
	}
	return h.flairTranscript(ctx, keep, reader)
}

func (h *APIHandler) contextRecent(ctx context.Context, room, reader string, n int) string {
	if n <= 0 || n > 60 {
		n = 20
	}
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, n)
	out := []json.RawMessage{}
	for _, row := range rows {
		out = append(out, row.Data)
	}
	return h.flairTranscript(ctx, out, reader)
}

// contextThread is everything a coworker said or was told in a conversation.
func (h *APIHandler) contextThread(ctx context.Context, room, reader string, members []string, handle string) string {
	target := ""
	for id, hd := range h.roomHandles(ctx, members) {
		if strings.EqualFold(hd, handle) {
			target = id
		}
	}
	if strings.EqualFold(handle, "owner") || strings.EqualFold(handle, "you") {
		target = "you"
	}
	if target == "" {
		return fmt.Sprintf("No one here is @%s.\n", handle)
	}
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 1000)
	keep := []json.RawMessage{}
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Data, &m) == nil && (m.Sender == target || containsID(m.Mentions, target)) {
			keep = append(keep, row.Data)
		}
	}
	if len(keep) > 30 {
		keep = keep[len(keep)-30:]
	}
	return h.flairTranscript(ctx, keep, reader)
}

func (h *APIHandler) contextSearch(ctx context.Context, room, reader, query string) string {
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return "Give words to search for.\n"
	}
	rows, _, _ := h.store.Messages(ctx, room, 0, 0, 1000)
	keep := []json.RawMessage{}
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Data, &m) != nil || m.Kind == "note" {
			continue
		}
		text := strings.ToLower(m.Text)
		hit := true
		for _, w := range words {
			hit = hit && strings.Contains(text, w)
		}
		if hit {
			keep = append(keep, row.Data)
		}
	}
	if len(keep) > 20 {
		keep = keep[len(keep)-20:]
	}
	return h.flairTranscript(ctx, keep, reader)
}

// flairTranscript is transcript with each message's flair and role claim, so
// fetched context reads like the turn it came from.
func (h *APIHandler) flairTranscript(ctx context.Context, rows []json.RawMessage, reader string) string {
	var t strings.Builder
	for _, b := range rows {
		var m Message
		if json.Unmarshal(b, &m) != nil || m.Kind == "note" {
			continue
		}
		tags := ""
		if m.Flair != "" {
			tags += "[" + m.Flair + "] "
		}
		if m.Role != "" {
			tags += "[role: " + m.Role + "] "
		}
		fmt.Fprintf(&t, "%s: %s%s\n", h.memberName(ctx, m.Sender), tags, m.Text)
		if m.File != "" {
			t.WriteString(h.fileLine(ctx, m, reader))
		}
	}
	return t.String()
}
