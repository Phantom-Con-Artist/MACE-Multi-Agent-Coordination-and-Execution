package api

import (
	"context"
	"encoding/json"
	"errors"
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
	ID           string    `json:"id"`
	Status       string    `json:"status"`
	Replies      int       `json:"replies"`
	Policy       Policy    `json:"policy"`
	Error        string    `json:"error,omitempty"`
	Updated      time.Time `json:"updated"`
	ActiveMember string    `json:"active_member,omitempty"`
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
	h.runMu.Lock()
	cancel := h.runs[id]
	if cancel != nil {
		cancel(nil)
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
func (h *APIHandler) beginConversation(room string, members []string, m Message) error {
	if len(members) == 0 {
		return fmt.Errorf("add an agent to this group first")
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	if h.runs[room] != nil {
		return fmt.Errorf("conversation is still running; pause it before sending a new task")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	h.runs[room] = cancel
	p := Policy{Mode: "reply", Limit: len(members)}
	h.store.Document(ctx, "policy", room, &p)
	run := Run{ID: room, Status: "running", Policy: p, Updated: time.Now().UTC()}
	if e := h.store.PutDocument(ctx, "run", room, run); e != nil {
		delete(h.runs, room)
		cancel(nil)
		return e
	}
	go func() {
		defer cancel(nil)
		defer func() { h.runMu.Lock(); delete(h.runs, room); delete(h.activeTurns, room); h.runMu.Unlock() }()
		h.audit("you", "CONVERSATION_STARTED", room, map[string]any{"mode": p.Mode, "limit": p.Limit})
		finish := func(status string, e error) {
			if status == "paused" && e == nil {
				if why := interruption(context.Cause(ctx)); why != "" {
					status, e = "interrupted", errors.New(why)
					note := Message{ID: newID(), Room: room, Sender: "mace", Kind: "system", Text: why, At: time.Now().UTC(), Delivery: map[string]string{}}
					h.store.PutDocument(context.Background(), "message:"+room, note.ID, note)
				}
			}
			run.ActiveMember = ""
			run.Status = status
			run.Error = ""
			if e != nil {
				run.Error = e.Error()
			}
			run.Updated = time.Now().UTC()
			h.store.PutDocument(context.Background(), "run", room, run)
			h.audit("orchestrator", "CONVERSATION_"+strings.ToUpper(status), room, map[string]any{"replies": run.Replies, "error": run.Error})
		}
		linked := map[string]bool{}
		lastSpeaker := ""
		lastReply := ""
		for index := 0; ; index++ {
			if ctx.Err() != nil {
				finish("paused", nil)
				return
			}
			if p.Mode != "continuous" && run.Replies >= p.Limit {
				finish("limit_reached", nil)
				return
			}
			var fresh Room
			if h.store.Document(ctx, "room", room, &fresh) == nil {
				if fresh.Removed {
					finish("paused", nil)
					return
				}
				members = fresh.Members
			}
			eligible := []string{}
			for _, id := range members {
				if run.Replies == 0 && id == m.Sender {
					continue
				}
				if h.available(ctx, id) {
					eligible = append(eligible, id)
				}
			}
			if len(eligible) == 0 {
				if len(members) == 1 && members[0] == m.Sender {
					finish("completed", nil)
				} else {
					finish("paused", fmt.Errorf("No agents are available. Resume an agent or add a member to continue."))
				}
				return
			}
			member := eligible[0]
			for i, id := range eligible {
				if id == lastSpeaker {
					member = eligible[(i+1)%len(eligible)]
					break
				}
			}
			for _, id := range eligible {
				if id != lastSpeaker && strings.Contains(strings.ToLower(lastReply), "@"+strings.ToLower(h.memberName(ctx, id))) {
					member = id
					break
				}
			}
			history, e := h.store.Documents(ctx, "message:"+room)
			if e != nil {
				finish("failed", e)
				return
			}
			prompt := h.conversationPrompt(ctx, room, member, members, p, history)
			turnCtx, turnCancel := context.WithCancel(ctx)
			h.runMu.Lock()
			h.activeTurns[room] = activeTurn{Member: member, Cancel: turnCancel}
			h.runMu.Unlock()
			run.ActiveMember = member
			run.Updated = time.Now().UTC()
			h.store.PutDocument(ctx, "run", room, run)
			var reply string
			if strings.Contains(member, "::") {
				parts := strings.SplitN(member, "::", 2)
				key := parts[0] + ":" + m.File
				if m.File != "" && !linked[key] {
					file, e := h.transfer(turnCtx, parts[0], m.File)
					if e != nil {
						turnCancel()
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
				reply, e = h.peerTurn(turnCtx, parts[0], parts[1], prompt)
			} else if strings.HasPrefix(member, "session_") {
				reply, e = h.turn(turnCtx, member, prompt, "local")
			} else {
				e = h.supervisor.SendInput(member, prompt+"\n")
				if e == nil {
					reply = "Task delivered to native process; inspect its output in the chat."
				}
			}
			interrupted := turnCtx.Err() != nil
			turnCancel()
			h.runMu.Lock()
			delete(h.activeTurns, room)
			h.runMu.Unlock()
			if ctx.Err() != nil {
				finish("paused", nil)
				return
			}
			if e != nil {
				if interrupted || !h.available(ctx, member) {
					continue
				}
				finish("failed", e)
				return
			}
			goalComplete := p.Mode == "goal" && strings.Contains(reply, "[GOAL_COMPLETE]")
			visibleReply := reply
			if goalComplete {
				visibleReply = strings.TrimSpace(strings.ReplaceAll(reply, "[GOAL_COMPLETE]", ""))
			}
			response := Message{ID: newID(), Room: room, Sender: member, Text: visibleReply, At: time.Now().UTC(), Delivery: map[string]string{}}
			if e := h.store.PutDocument(ctx, "message:"+room, response.ID, response); e != nil {
				finish("failed", e)
				return
			}
			run.Replies++
			run.ActiveMember = ""
			lastSpeaker = member
			lastReply = reply
			run.Updated = time.Now().UTC()
			h.store.PutDocument(ctx, "run", room, run)
			if goalComplete {
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
