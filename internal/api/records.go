package api

import (
	"context"
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"time"

	"mace/internal/records"
	"mace/internal/storage"
)

// Every message is also written as a hashed, signed record owned by this node
// (docs/DESIGN-HUB-WORKER.md). Messages stay the source the interface reads
// until the hub merges records from many nodes; the record ID is kept on the
// message so it can be fetched by hash.

// openRecords loads the node identity kept beside the database and backfills
// records for messages written before records existed.
func openRecords(store *storage.Store) *records.Log {
	ctx := context.Background()
	var id *records.Identity
	if store.Path() == ":memory:" {
		id = records.NewIdentity()
	} else {
		var err error
		if id, err = records.LoadIdentity(filepath.Join(filepath.Dir(store.Path()), "node.key")); err != nil {
			log.Printf("[MACE] Records are off: the node key could not be loaded: %v", err)
			return nil
		}
	}
	l, err := records.NewLog(ctx, id, store)
	if err != nil {
		log.Printf("[MACE] Records are off: %v", err)
		return nil
	}
	added := 0
	err = store.EachMessage(ctx, func(room string, row storage.StoredMessage) error {
		var m Message
		if json.Unmarshal(row.Data, &m) != nil || m.Record != "" {
			return nil
		}
		r, err := l.Append(ctx, messageEntry(&m))
		if err != nil {
			return err
		}
		m.Record = r.ID
		added++
		return store.UpdateMessage(ctx, m.ID, m)
	})
	if err != nil {
		log.Printf("[MACE] Writing records for earlier messages stopped: %v", err)
	} else if added > 0 {
		log.Printf("[MACE] Wrote records for %d earlier messages. Node %s.", added, id.ID)
	}
	return l
}

// messageEntry is the lasting content of a message. Delivery state and
// storage positions are local and change, so they are left out.
func messageEntry(m *Message) records.Entry {
	kind := m.Kind
	if kind == "" {
		kind = "message"
	}
	return records.Entry{
		Conversation: m.Room,
		Author:       m.Sender,
		Kind:         kind,
		Ref:          m.ID,
		At:           m.At,
		Body: map[string]any{
			"message":      m.ID,
			"text":         m.Text,
			"file":         m.File,
			"mentions":     m.Mentions,
			"mentions_all": m.MentionsAll,
			"hop":          m.Hop,
			"wake":         m.Wake,
			"activity":     m.Activity,
		},
	}
}

// recordMessage writes a message's record and keeps its ID on the message.
func (h *APIHandler) recordMessage(ctx context.Context, m *Message) {
	if h.records == nil {
		return
	}
	r, err := h.records.Append(ctx, messageEntry(m))
	if err != nil {
		log.Printf("[MACE] Could not write the record for message %s: %v", m.ID, err)
		return
	}
	m.Record = r.ID
	h.store.UpdateMessage(ctx, m.ID, m)
}

// One conversation with an agent can live on two machines: the owner talks
// to a worker's agent from the hub (the hub's conversation "<worker>::<id>")
// and directly on the worker (the worker's conversation "<id>"). Each machine
// writes its own records; synced records from the other side are merged into
// the chat as read-only messages marked with the machine they came from.

// counterpart names the other machine's side of a private chat with an
// agent: its node ID and its conversation ID.
func (h *APIHandler) counterparts(ctx context.Context, room string) map[string]string {
	out := map[string]string{}
	if node, id, remote := strings.Cut(room, "::"); remote {
		var n Node
		if h.store.Document(ctx, "node", node, &n) == nil {
			out[node] = id
		}
		return out
	}
	var s Session
	if h.store.Document(ctx, "session", room, &s) != nil {
		return out
	}
	self := h.selfNode().ID
	for _, n := range h.knownNodes(ctx) {
		out[n.ID] = self + "::" + room
	}
	return out
}

// foreignMessages are the other machines' records of a private chat with an
// agent, as messages in this machine's terms.
func (h *APIHandler) foreignMessages(ctx context.Context, room string) []Message {
	out := []Message{}
	if h.records == nil {
		return out
	}
	local, agent, remote := strings.Cut(room, "::")
	for node, conversation := range h.counterparts(ctx, room) {
		list, err := h.records.Conversation(ctx, conversation)
		if err != nil {
			continue
		}
		var n Node
		h.store.Document(ctx, "node", node, &n)
		for _, rec := range list {
			if rec.Node != node || rec.Kind != "message" {
				continue
			}
			var body struct {
				Message  string     `json:"message"`
				Text     string     `json:"text"`
				File     string     `json:"file"`
				Activity []Activity `json:"activity"`
			}
			if json.Unmarshal(rec.Body, &body) != nil || body.Message == "" {
				continue
			}
			sender := rec.Author
			switch {
			case remote && sender == agent:
				sender = local + "::" + agent // the worker's agent, seen from the hub
			case !remote && strings.HasSuffix(sender, "::"+room):
				sender = room // the hub's name for this machine's agent
			}
			text := body.Text
			if body.File != "" {
				text = strings.TrimSpace(text + "\n(file on " + n.Name + ": " + filepath.Base(body.File) + ")")
			}
			out = append(out, Message{ID: body.Message, Room: room, Sender: sender, Text: text, Activity: body.Activity, At: time.UnixMilli(rec.Clock.Wall).UTC(), Origin: n.Name, Record: rec.ID, Delivery: map[string]string{}})
		}
	}
	return out
}
