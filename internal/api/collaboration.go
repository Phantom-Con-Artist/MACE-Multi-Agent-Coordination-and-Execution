package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type Policy struct {
	Mode  string `json:"mode"`
	Goal  string `json:"goal"`
	Limit int    `json:"limit"`
	// Tone is "casual" (the default: short, chat-style, shorthand welcome)
	// or "professional" (short and plain, no slang).
	Tone string `json:"tone,omitempty"`
	// Lead is the member who runs a team ("team" mode, shown as Goal): the
	// owner's unaddressed messages go to the lead, workers report to it, and
	// only its [done] finishes the run.
	Lead string `json:"lead,omitempty"`
}
type Run struct {
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Replies      int       `json:"replies"`
	Policy       Policy    `json:"policy"`
	Error        string    `json:"error,omitempty"`
	Updated      time.Time `json:"updated"`
	ActiveMember string    `json:"active_member,omitempty"`
	// ActiveMembers lists every agent working in this conversation right now.
	ActiveMembers []string `json:"active_members,omitempty"`
}
type FileLink struct {
	Peer        string `json:"peer"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (h *APIHandler) policy(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "room")
	if r.Method == "GET" {
		p := Policy{Mode: "reply", Limit: 1}
		var room Room
		if h.store.Document(r.Context(), "room", id, &room) == nil && len(room.Members) > 0 {
			p.Limit = len(room.Members)
		}
		h.store.Document(r.Context(), "policy", id, &p)
		writeJSON(w, 200, p)
		return
	}
	var p Policy
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&p) != nil {
		http.Error(w, "invalid policy", 400)
		return
	}
	if p.Mode != "reply" && p.Mode != "goal" && p.Mode != "continuous" && p.Mode != "team" {
		http.Error(w, "unknown conversation mode", 400)
		return
	}
	if p.Tone != "" && p.Tone != "casual" && p.Tone != "professional" {
		http.Error(w, "tone must be casual or professional", 400)
		return
	}
	if p.Mode == "goal" && strings.TrimSpace(p.Goal) == "" {
		http.Error(w, "set a goal first", 400)
		return
	}
	if p.Mode == "team" {
		var room Room
		if h.store.Document(r.Context(), "room", id, &room) != nil || !containsID(room.Members, p.Lead) {
			http.Error(w, "Choose a lead from this group's members.", 400)
			return
		}
	} else {
		p.Lead = ""
	}
	if p.Mode != "continuous" && p.Mode != "team" && (p.Limit < 1 || p.Limit > 100) {
		http.Error(w, "reply limit must be between 1 and 100", 400)
		return
	}
	if e := h.store.PutDocument(r.Context(), "policy", id, p); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "POLICY_CHANGED", id, map[string]any{"mode": p.Mode, "limit": p.Limit, "goal": p.Goal})
	writeJSON(w, 200, p)
}
func (h *APIHandler) runsList(w http.ResponseWriter, r *http.Request) {
	v, e := h.store.Documents(r.Context(), "run")
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	visible := []json.RawMessage{}
	for _, b := range v {
		var run Run
		json.Unmarshal(b, &run)
		var room Room
		var session Session
		if h.store.Document(r.Context(), "room", run.ID, &room) == nil && room.Removed {
			continue
		}
		if h.store.Document(r.Context(), "session", run.ID, &session) == nil && session.Status == "removed" {
			continue
		}
		visible = append(visible, b)
	}
	writeJSON(w, 200, visible)
}
func (h *APIHandler) pause(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "room")
	stopped := h.pauseRoom(id, nil)
	h.audit("you", "CONVERSATION_PAUSE", id, nil)
	writeJSON(w, 200, map[string]bool{"pausing": stopped})
}
func (h *APIHandler) knownMember(ctx context.Context, id string) bool {
	if strings.Contains(id, "::") {
		parts := strings.SplitN(id, "::", 2)
		var p Peer
		return h.store.Document(ctx, "peer", parts[0], &p) == nil && parts[1] != ""
	}
	var s Session
	if h.store.Document(ctx, "session", id, &s) == nil {
		return s.Status != "removed"
	}
	p, e := h.supervisor.GetAgent(id)
	return e == nil && p != nil
}
func (h *APIHandler) managed(members []string) bool {
	for _, id := range members {
		if strings.HasPrefix(id, "session_") || strings.Contains(id, "::") {
			return true
		}
	}
	return false
}
