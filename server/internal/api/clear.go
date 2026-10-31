package api

import (
	"context"
	"net/http"
	"time"
)

// clearedAt is when a conversation's history was last cleared; zero if never.
// Messages of agents on other machines come back from their records, so
// anything older than this is hidden when they are listed.
func (h *APIHandler) clearedAt(room string) time.Time {
	var d struct {
		At time.Time `json:"at"`
	}
	h.store.Document(context.Background(), "cleared", room, &d)
	return d.At
}

// clearChat serves POST /api/v1/chats/{room}/clear. It deletes the messages
// stored on this machine and hides older messages that arrive from other
// machines' records. The agent keeps its own native memory; /new starts it
// fresh. The audit log notes that a clear happened.
func (h *APIHandler) clearChat(w http.ResponseWriter, r *http.Request) {
	room := routeParam(r, "room")
	if _, _, err := h.conversation(r.Context(), room); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if e := h.store.PutDocument(r.Context(), "cleared", room, map[string]time.Time{"at": time.Now().UTC()}); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	n, e := h.store.DeleteMessages(r.Context(), room)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "CHAT_CLEARED", room, map[string]any{"messages": n})
	h.events.publish("reset", map[string]any{"room": room})
	writeJSON(w, 200, map[string]any{"cleared": n})
}
