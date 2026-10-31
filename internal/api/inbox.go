package api

import (
	"encoding/json"
	"net/http"
	"strings"
)

// inbox serves POST /api/v1/inbox. The browser sends the last sequence it has
// seen per conversation; the reply lists each conversation's newest message
// and the unread count since then.
func (h *APIHandler) inbox(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Seen map[string]int64 `json:"seen"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	rows, err := h.store.Inbox(r.Context(), req.Seen)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type entry struct {
		Room   string  `json:"room"`
		Latest Message `json:"latest"`
		Unread int     `json:"unread"`
	}
	out := []entry{}
	for _, row := range rows {
		var m Message
		if json.Unmarshal(row.Latest.Data, &m) != nil {
			continue
		}
		m.Seq, m.Activity = row.Latest.Seq, nil
		out = append(out, entry{Room: row.Room, Latest: m, Unread: row.Unread})
	}
	writeJSON(w, 200, out)
}

// receipts serves GET /api/v1/chats/{room}/receipts: how far each agent has
// actually read, by message sequence.
func (h *APIHandler) receipts(w http.ResponseWriter, r *http.Request) {
	room := routeParam(r, "room")
	members, _, err := h.conversation(r.Context(), room)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	out := map[string]int64{}
	for _, m := range members {
		if strings.TrimSpace(m) != "" {
			out[m] = h.cursor(r.Context(), m, room)
		}
	}
	writeJSON(w, 200, out)
}
