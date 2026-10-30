package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Room struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Members []string          `json:"members"`
	Roles   map[string]string `json:"roles,omitempty"`
	Removed bool              `json:"removed,omitempty"`
}
type Message struct {
	ID       string            `json:"id"`
	Room     string            `json:"room"`
	Sender   string            `json:"sender"`
	Text     string            `json:"text"`
	File     string            `json:"file,omitempty"`
	At       time.Time         `json:"at"`
	Delivery map[string]string `json:"delivery"`
}

func newID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
func (h *APIHandler) rooms(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "storage unavailable", 503)
		return
	}
	if r.Method == "GET" {
		v, e := h.store.Documents(r.Context(), "room")
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		visible := []json.RawMessage{}
		for _, b := range v {
			var room Room
			if json.Unmarshal(b, &room) == nil && !room.Removed {
				visible = append(visible, b)
			}
		}
		writeJSON(w, 200, visible)
		return
	}
	var room Room
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&room) != nil || strings.TrimSpace(room.Name) == "" || len(room.Members) == 0 {
		http.Error(w, "name and members required", 400)
		return
	}
	for _, id := range room.Members {
		if !h.knownMember(r.Context(), id) {
			http.Error(w, "unknown member", 400)
			return
		}
	}
	room.ID = newID()
	if e := h.store.PutDocument(r.Context(), "room", room.ID, room); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 201, room)
}
func (h *APIHandler) messages(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "storage unavailable", 503)
		return
	}
	id := routeParam(r, "room")
	if r.Method == "GET" {
		v, e := h.store.Documents(r.Context(), "message:"+id)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	var m Message
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&m) != nil || (strings.TrimSpace(m.Text) == "" && m.File == "") {
		http.Error(w, "message required", 400)
		return
	}
	members := []string{id}
	found := false
	rooms, e := h.store.Documents(r.Context(), "room")
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	for _, b := range rooms {
		var room Room
		json.Unmarshal(b, &room)
		if room.ID == id {
			if room.Removed {
				http.Error(w, "This group was removed.", http.StatusGone)
				return
			}
			members = room.Members
			found = true
			break
		}
	}
	if !found {
		if !h.knownMember(r.Context(), id) {
			http.Error(w, "conversation not found", 404)
			return
		}
	}
	// Agent-originated messages must identify a member and explicitly select recipients.
	if m.Sender != "" && m.Sender != "you" {
		allowed := false
		for _, v := range members {
			if v == m.Sender {
				allowed = true
			}
		}
		if !allowed {
			http.Error(w, "sender is not a member", 403)
			return
		}
	} else {
		m.Sender = "you"
	}
	m.ID = newID()
	m.Room = id
	m.At = time.Now().UTC()
	m.Delivery = map[string]string{}
	if m.File != "" {
		root, e := os.OpenRoot(h.fileRoot)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		f, e := root.Open(m.File)
		root.Close()
		if e != nil {
			http.Error(w, "file outside shared root or unavailable", 400)
			return
		}
		f.Close()
	}
	if e := h.store.PutDocument(r.Context(), "message:"+id, m.ID, m); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	if h.managed(members) {
		if e := h.beginConversation(id, members, m); e != nil {
			m.Delivery["conversation"] = e.Error()
			h.store.PutDocument(r.Context(), "message:"+id, m.ID, m)
			writeJSON(w, 201, m)
			return
		}
		m.Delivery["conversation"] = "delivered"
		h.store.PutDocument(r.Context(), "message:"+id, m.ID, m)
		h.audit(m.Sender, "MESSAGE_SENT", id, map[string]any{"message": m.ID, "file": m.File})
		writeJSON(w, 201, m)
		return
	}
	for _, target := range members {
		if target == m.Sender {
			continue
		}
		text := m.Text
		if m.File != "" {
			text += "\n[Shared file: " + filepath.Join(h.fileRoot, m.File) + "]"
		}
		if found {
			text = fmt.Sprintf("[MACE room %s | from %s]\n%s", id, m.Sender, text)
		}
		err := h.supervisor.SendInput(target, text+"\n")
		if err != nil {
			m.Delivery[target] = err.Error()
		} else {
			m.Delivery[target] = "delivered"
		}
	}
	if e := h.store.PutDocument(r.Context(), "message:"+id, m.ID, m); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 201, m)
}
func (h *APIHandler) files(w http.ResponseWriter, r *http.Request) {
	root, err := os.OpenRoot(h.fileRoot)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer root.Close()
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		path, err = filepath.Rel(h.fileRoot, path)
		if err != nil {
			http.Error(w, "invalid path", 400)
			return
		}
	}
	f, err := root.Open(path)
	if err != nil {
		http.Error(w, "file unavailable or outside shared root", 404)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if info.IsDir() {
		entries, err := f.ReadDir(-1)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		result := []map[string]any{}
		for _, entry := range entries {
			i, e := entry.Info()
			if e != nil {
				continue
			}
			result = append(result, map[string]any{"name": entry.Name(), "directory": entry.IsDir(), "size": i.Size(), "path": filepath.ToSlash(filepath.Join(path, entry.Name()))})
		}
		writeJSON(w, 200, result)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}
