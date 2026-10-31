package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (h *APIHandler) removeGroup(w http.ResponseWriter, r *http.Request) {
	h.roomMu.Lock()
	defer h.roomMu.Unlock()
	id := routeParam(r, "room")
	var group Room
	if h.store.Document(r.Context(), "room", id, &group) != nil {
		http.Error(w, "Group not found.", 404)
		return
	}
	group.Removed = true
	if e := h.store.PutDocument(r.Context(), "room", id, group); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.runMu.Lock()
	if cancel := h.runs[id]; cancel != nil {
		cancel(nil)
	}
	h.runMu.Unlock()
	h.audit("you", "GROUP_REMOVED", id, map[string]any{"history_retained": true})
	writeJSON(w, 200, map[string]bool{"removed": true})
}
func (h *APIHandler) memberRole(w http.ResponseWriter, r *http.Request) {
	h.roomMu.Lock()
	defer h.roomMu.Unlock()
	id, member := routeParam(r, "room"), routeParam(r, "member")
	var group Room
	if h.store.Document(r.Context(), "room", id, &group) != nil || group.Removed {
		http.Error(w, "Group not found.", 404)
		return
	}
	found := false
	for _, m := range group.Members {
		if m == member {
			found = true
		}
	}
	if !found {
		http.Error(w, "That agent is not in this group.", 404)
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || len(req.Role) > 2000 {
		http.Error(w, "Use a role description under 2000 characters.", 400)
		return
	}
	if group.Roles == nil {
		group.Roles = map[string]string{}
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		delete(group.Roles, member)
	} else {
		group.Roles[member] = role
	}
	if e := h.store.PutDocument(r.Context(), "room", id, group); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "MEMBER_ROLE_CHANGED", id, map[string]any{"member": member, "role": role})
	writeJSON(w, 200, group)
}
func (h *APIHandler) startDiscussion(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "room")
	var group Room
	if h.store.Document(r.Context(), "room", id, &group) != nil || group.Removed {
		http.Error(w, "Group not found.", 404)
		return
	}
	if len(group.Members) == 0 {
		http.Error(w, "Add an agent before starting the conversation.", 400)
		return
	}
	p := Policy{}
	if h.store.Document(r.Context(), "policy", id, &p) != nil || p.Mode != "continuous" {
		http.Error(w, "Choose Continuous mode to start an open conversation.", 400)
		return
	}
	m := Message{ID: newID(), Room: id, Sender: "mace", Text: "Continuous conversation started. The team can talk and collaborate without waiting for another message.", At: time.Now().UTC(), Delivery: map[string]string{}}
	if e := h.beginConversation(id, group.Members, m); e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	if e := h.store.PutDocument(r.Context(), "message:"+id, m.ID, m); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 201, m)
}

// Stopping one employee does not stop the rest of a group. Membership edits are
// observed on the next turn, and an in-flight reply from that member is cancelled.
func (h *APIHandler) stopMemberRuns(id string, remove bool) {
	h.roomMu.Lock()
	defer h.roomMu.Unlock()
	h.runMu.Lock()
	if cancel := h.runs[id]; cancel != nil {
		cancel(nil)
	}
	h.runMu.Unlock()
	raw, _ := h.store.Documents(context.Background(), "room")
	for _, b := range raw {
		var group Room
		json.Unmarshal(b, &group)
		found := false
		members := []string{}
		for _, member := range group.Members {
			if member == id {
				found = true
			} else {
				members = append(members, member)
			}
		}
		if !found {
			continue
		}
		if remove {
			group.Members = members
			delete(group.Roles, id)
			h.store.PutDocument(context.Background(), "room", group.ID, group)
		}
		h.cancelMemberTurn(group.ID, id)
	}
}
