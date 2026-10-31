package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"mace/internal/storage"
)

// Log writes this node's records and reads the records it holds, its own and
// cached ones from other nodes.
type Log struct {
	id    *Identity
	store *storage.Store
	clock HLC
	mu    sync.Mutex
}

// Entry is what a caller supplies; the log fills in the node, clock, chain
// link, and the other nodes' heads it has seen.
type Entry struct {
	Conversation string
	Author       string
	Kind         string
	// Ref names what the record was written for, such as a message ID. A
	// second write with the same Ref returns the first record.
	Ref  string
	Body any
	// At keeps an original time when existing data is turned into records.
	At time.Time
}

// NewLog opens the log for a node and restores its clock from what it wrote.
func NewLog(ctx context.Context, id *Identity, store *storage.Store) (*Log, error) {
	l := &Log{id: id, store: store}
	wall, count, err := store.LatestRecordClock(ctx, id.ID)
	if err != nil {
		return nil, err
	}
	l.clock.Observe(Clock{Wall: wall, Count: count})
	return l, nil
}

// Identity returns the node identity the log signs with.
func (l *Log) Identity() *Identity { return l.id }

// Observe moves the clock past a record seen from another node.
func (l *Log) Observe(c Clock) { l.clock.Observe(c) }

// Append seals and stores a record at the end of this node's chain in the
// entry's conversation.
func (l *Log) Append(ctx context.Context, e Entry) (Record, error) {
	if e.Conversation == "" || e.Kind == "" {
		return Record{}, errors.New("a record needs a conversation and a kind")
	}
	body, err := json.Marshal(e.Body)
	if err != nil {
		return Record{}, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Ref != "" {
		if id, err := l.store.RecordIDByRef(ctx, l.id.ID, e.Ref); err != nil {
			return Record{}, err
		} else if id != "" {
			return l.read(ctx, e.Conversation, id)
		}
	}
	heads, err := l.store.ConversationHeads(ctx, e.Conversation)
	if err != nil {
		return Record{}, err
	}
	prev := heads[l.id.ID]
	seen := []string{}
	for node, head := range heads {
		if node != l.id.ID {
			seen = append(seen, head)
		}
	}
	var clock Clock
	if e.At.IsZero() {
		clock = l.clock.Now()
	} else {
		clock = l.clock.Stamp(e.At.UnixMilli())
	}
	r, err := l.id.Seal(Record{Node: l.id.ID, Author: e.Author, Conversation: e.Conversation, Clock: clock, Prev: prev, Seen: seen, Kind: e.Kind, Body: body})
	if err != nil {
		return Record{}, err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return Record{}, err
	}
	row := storage.RecordRow{ID: r.ID, Node: r.Node, Conversation: r.Conversation, Wall: r.Clock.Wall, Count: r.Clock.Count, Prev: r.Prev, Kind: r.Kind, Ref: e.Ref, Data: data}
	if err := l.store.AppendRecord(ctx, row, prev); err != nil {
		return Record{}, err
	}
	return r, nil
}

// Conversation returns every record held for a conversation, in causal order.
func (l *Log) Conversation(ctx context.Context, conversation string) ([]Record, error) {
	rows, err := l.store.ConversationRecords(ctx, conversation)
	if err != nil {
		return nil, err
	}
	list := make([]Record, 0, len(rows))
	for _, row := range rows {
		var r Record
		if err := json.Unmarshal(row.Data, &r); err != nil {
			return nil, fmt.Errorf("stored record %s is unreadable: %w", short(row.ID), err)
		}
		list = append(list, r)
	}
	return Order(list), nil
}

func (l *Log) read(ctx context.Context, conversation, id string) (Record, error) {
	list, err := l.Conversation(ctx, conversation)
	if err != nil {
		return Record{}, err
	}
	for _, r := range list {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("record %s is not in conversation %s", short(id), conversation)
}
