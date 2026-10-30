package api

import (
	"encoding/json"
	"net/http"
)

func (h *APIHandler) addMember(w http.ResponseWriter, r *http.Request) {
	var room Room
	if h.store.Document(r.Context(), "room", routeParam(r, "room"), &room) != nil {
		http.Error(w, "group unavailable", 404)
		return
	}
	var req struct {
		Member string `json:"member"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || !h.knownMember(r.Context(), req.Member) {
		http.Error(w, "unknown agent", 400)
		return
	}
	for _, m := range room.Members {
		if m == req.Member {
			writeJSON(w, 200, room)
			return
		}
	}
	room.Members = append(room.Members, req.Member)
	if e := h.store.PutDocument(r.Context(), "room", room.ID, room); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "MEMBER_ADDED", room.ID, map[string]any{"member": req.Member})
	writeJSON(w, 200, room)
}
