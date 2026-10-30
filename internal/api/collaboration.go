package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Policy struct {
	Mode  string `json:"mode"`
	Goal  string `json:"goal"`
	Limit int    `json:"limit"`
}
type Run struct {
	ID      string    `json:"id"`
	Status  string    `json:"status"`
	Replies int       `json:"replies"`
	Policy  Policy    `json:"policy"`
	Error   string    `json:"error,omitempty"`
	Updated time.Time `json:"updated"`
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
	if p.Mode != "reply" && p.Mode != "goal" && p.Mode != "continuous" {
		http.Error(w, "unknown conversation mode", 400)
		return
	}
	if p.Mode == "goal" && strings.TrimSpace(p.Goal) == "" {
		http.Error(w, "set a goal first", 400)
		return
	}
	if p.Mode != "continuous" && (p.Limit < 1 || p.Limit > 100) {
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
	writeJSON(w, 200, v)
}
func (h *APIHandler) pause(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "room")
	h.runMu.Lock()
	cancel := h.runs[id]
	if cancel != nil {
		cancel()
	}
	h.runMu.Unlock()
	h.audit("you", "CONVERSATION_PAUSE", id, nil)
	writeJSON(w, 200, map[string]bool{"pausing": cancel != nil})
}
func (h *APIHandler) knownMember(ctx context.Context, id string) bool {
	if strings.Contains(id, "::") {
		parts := strings.SplitN(id, "::", 2)
		var p Peer
		return h.store.Document(ctx, "peer", parts[0], &p) == nil && parts[1] != ""
	}
	var s Session
	if h.store.Document(ctx, "session", id, &s) == nil {
		return true
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
func (h *APIHandler) beginConversation(room string, members []string, m Message) error {
	if len(members) == 0 {
		return fmt.Errorf("add an agent to this group first")
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	if h.runs[room] != nil {
		return fmt.Errorf("conversation is still running; pause it before sending a new task")
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.runs[room] = cancel
	p := Policy{Mode: "reply", Limit: len(members)}
	h.store.Document(ctx, "policy", room, &p)
	run := Run{ID: room, Status: "running", Policy: p, Updated: time.Now().UTC()}
	if e := h.store.PutDocument(ctx, "run", room, run); e != nil {
		delete(h.runs, room)
		cancel()
		return e
	}
	go func() {
		defer cancel()
		defer func() { h.runMu.Lock(); delete(h.runs, room); h.runMu.Unlock() }()
		h.audit("you", "CONVERSATION_STARTED", room, map[string]any{"mode": p.Mode, "limit": p.Limit})
		finish := func(status string, e error) {
			run.Status = status
			if e != nil {
				run.Error = e.Error()
			}
			run.Updated = time.Now().UTC()
			h.store.PutDocument(context.Background(), "run", room, run)
			h.audit("orchestrator", "CONVERSATION_"+strings.ToUpper(status), room, map[string]any{"replies": run.Replies, "error": run.Error})
		}
		linked := map[string]bool{}
		for index := 0; ; index++ {
			if ctx.Err() != nil {
				finish("paused", nil)
				return
			}
			if p.Mode != "continuous" && run.Replies >= p.Limit {
				finish("limit_reached", nil)
				return
			}
			member := members[index%len(members)]
			if member == m.Sender {
				if len(members) == 1 {
					finish("completed", nil)
					return
				}
				continue
			}
			history, e := h.store.Documents(ctx, "message:"+room)
			if e != nil {
				finish("failed", e)
				return
			}
			var transcript strings.Builder
			transcript.WriteString("You are collaborating in a MACE conversation. Answer the latest task using the shared context. Do not invent other participants' replies.\n")
			if p.Mode == "goal" {
				transcript.WriteString("Goal: " + p.Goal + "\nOnly when the goal is met, include [GOAL_COMPLETE] and give evidence.\n")
			}
			start := len(history) - 40
			if start < 0 {
				start = 0
			}
			for _, b := range history[start:] {
				var item Message
				json.Unmarshal(b, &item)
				transcript.WriteString(item.Sender + ": " + item.Text + "\n")
				if item.File != "" {
					transcript.WriteString("Shared host file: " + h.fileRoot + "/" + item.File + "\n")
				}
			}
			prompt := truncate(transcript.String(), 100000)
			var reply string
			if strings.Contains(member, "::") {
				parts := strings.SplitN(member, "::", 2)
				key := parts[0] + ":" + m.File
				if m.File != "" && !linked[key] {
					file, e := h.transfer(ctx, parts[0], m.File)
					if e != nil {
						finish("failed", e)
						return
					}
					if e := h.store.PutDocument(ctx, "filelinks:"+room, key, FileLink{Peer: parts[0], Source: m.File, Destination: file}); e != nil {
						finish("failed", e)
						return
					}
					linked[key] = true
				}
				links, _ := h.store.Documents(ctx, "filelinks:"+room)
				for _, b := range links {
					var link FileLink
					json.Unmarshal(b, &link)
					if link.Peer == parts[0] {
						prompt += "\nUse your local copy of " + link.Source + ": " + link.Destination
					}
				}
				reply, e = h.peerTurn(ctx, parts[0], parts[1], prompt)
			} else if strings.HasPrefix(member, "session_") {
				reply, e = h.turn(ctx, member, prompt)
			} else {
				e = h.supervisor.SendInput(member, prompt+"\n")
				if e == nil {
					reply = "Task delivered to native process; inspect its output in the chat."
				}
			}
			if ctx.Err() != nil {
				finish("paused", nil)
				return
			}
			if e != nil {
				finish("failed", e)
				return
			}
			response := Message{ID: newID(), Room: room, Sender: member, Text: reply, At: time.Now().UTC(), Delivery: map[string]string{}}
			if e := h.store.PutDocument(ctx, "message:"+room, response.ID, response); e != nil {
				finish("failed", e)
				return
			}
			run.Replies++
			run.Updated = time.Now().UTC()
			h.store.PutDocument(ctx, "run", room, run)
			if p.Mode == "goal" && strings.Contains(reply, "[GOAL_COMPLETE]") {
				finish("goal_reported_complete", nil)
				return
			}
			select {
			case <-ctx.Done():
				finish("paused", nil)
				return
			case <-time.After(250 * time.Millisecond):
			}
		}
	}()
	return nil
}
