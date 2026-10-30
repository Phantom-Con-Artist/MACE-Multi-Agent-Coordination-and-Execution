package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (h *APIHandler) controlSession(w http.ResponseWriter, r *http.Request) {
	h.lifecycleMu.Lock()
	defer h.lifecycleMu.Unlock()
	id := routeParam(r, "id")
	var req struct {
		Action string `json:"action"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		http.Error(w, "action required", 400)
		return
	}
	var s Session
	if strings.Contains(id, "::") {
		parts := strings.SplitN(id, "::", 2)
		var p Peer
		if h.store.Document(r.Context(), "peer", parts[0], &p) != nil {
			http.Error(w, "host unavailable", 404)
			return
		}
		var out Session
		if e := h.peerRequest(r.Context(), p, "/sessions/"+parts[1]+"/control", req, &out); e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		cached := out
		cached.ID = id
		h.store.PutDocument(r.Context(), "remote-session", id, cached)
		if req.Action != "resume" {
			h.stopMemberRuns(id, req.Action == "remove")
		}
		h.audit("you", "REMOTE_AGENT_CONTROL", id, map[string]any{"action": req.Action})
		writeJSON(w, 200, out)
		return
	}
	if h.store.Document(r.Context(), "session", id, &s) != nil {
		http.Error(w, "session unavailable", 404)
		return
	}
	switch req.Action {
	case "stop", "terminate", "remove", "resume":
	default:
		http.Error(w, "unknown action", 400)
		return
	}
	if req.Action != "resume" {
		h.stopMemberRuns(id, req.Action == "remove")
		processes, _ := h.supervisor.ListAgents(r.Context())
		for _, p := range processes {
			if p.Labels["session"] == id && !p.Status.IsTerminal() {
				h.supervisor.StopAgent(p.ID, "SIGTERM", time.Second)
			}
		}
	}
	switch req.Action {
	case "resume":
		s.Status = "ready"
	case "stop":
		s.Status = "stopped"
	case "terminate":
		s.Status = "terminated"
	case "remove":
		s.Status = "removed"
	}
	if e := h.store.PutDocument(context.Background(), "session", id, s); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "AGENT_"+req.Action, id, nil)
	writeJSON(w, 200, s)
}
func (h *APIHandler) removeMember(w http.ResponseWriter, r *http.Request) {
	h.roomMu.Lock()
	defer h.roomMu.Unlock()
	var room Room
	if h.store.Document(r.Context(), "room", routeParam(r, "room"), &room) != nil {
		http.Error(w, "group unavailable", 404)
		return
	}
	id := routeParam(r, "member")
	members := []string{}
	for _, m := range room.Members {
		if m != id {
			members = append(members, m)
		}
	}
	room.Members = members
	delete(room.Roles, id)
	if e := h.store.PutDocument(r.Context(), "room", room.ID, room); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.cancelMemberTurn(room.ID, id)
	h.audit("you", "MEMBER_REMOVED", room.ID, map[string]any{"member": id})
	writeJSON(w, 200, room)
}
