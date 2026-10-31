package records

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mace/internal/storage"
)

func openLog(t *testing.T) (*Log, *storage.Store) {
	t.Helper()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	log, err := NewLog(context.Background(), NewIdentity(), store)
	if err != nil {
		t.Fatal(err)
	}
	return log, store
}

func TestLogChainsEachConversationAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	log, _ := openLog(t)
	a, err := log.Append(ctx, Entry{Conversation: "c", Author: "you", Kind: "message", Ref: "m1", Body: map[string]string{"text": "one"}})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := log.Append(ctx, Entry{Conversation: "c", Author: "agy", Kind: "message", Ref: "m2", Body: map[string]string{"text": "two"}})
	other, _ := log.Append(ctx, Entry{Conversation: "d", Author: "you", Kind: "message", Ref: "m3", Body: map[string]string{"text": "elsewhere"}})
	if a.Prev != "" || b.Prev != a.ID || other.Prev != "" {
		t.Fatalf("chains: a.prev=%q b.prev=%q other.prev=%q", a.Prev, b.Prev, other.Prev)
	}
	again, err := log.Append(ctx, Entry{Conversation: "c", Author: "you", Kind: "message", Ref: "m1", Body: map[string]string{"text": "one"}})
	if err != nil || again.ID != a.ID {
		t.Fatalf("writing the same message twice must return the first record: %v %s", err, again.ID)
	}
	got, err := log.Conversation(ctx, "c")
	if err != nil || len(got) != 2 || got[0].ID != a.ID || got[1].ID != b.ID {
		t.Fatalf("conversation: %v %+v", err, got)
	}
	if err := VerifyChain(got, log.Identity().Public); err != nil {
		t.Fatalf("stored chain should verify: %v", err)
	}
}

func TestLogSeesOtherNodesAndKeepsItsClockAcrossRestarts(t *testing.T) {
	ctx := context.Background()
	log, store := openLog(t)
	// A record cached from another node, written directly as sync will.
	pc := NewIdentity()
	foreign, _ := pc.Seal(Record{Node: pc.ID, Author: "agy", Conversation: "g", Clock: Clock{Wall: time.Now().Add(time.Hour).UnixMilli()}, Kind: "message", Body: json.RawMessage(`{"text":"from the pc"}`)})
	data, _ := json.Marshal(foreign)
	if err := store.AppendRecord(ctx, storage.RecordRow{ID: foreign.ID, Node: pc.ID, Conversation: "g", Wall: foreign.Clock.Wall, Prev: "", Kind: "message", Cached: true, Data: data}, ""); err != nil {
		t.Fatal(err)
	}
	log.Observe(foreign.Clock)
	mine, _ := log.Append(ctx, Entry{Conversation: "g", Author: "you", Kind: "message", Ref: "m1", Body: map[string]string{"text": "reply"}})
	if len(mine.Seen) != 1 || mine.Seen[0] != foreign.ID || !foreign.Clock.Less(mine.Clock) {
		t.Fatalf("a new record should have seen the other node's head and moved past its clock: %+v", mine)
	}

	restarted, err := NewLog(ctx, log.Identity(), store)
	if err != nil {
		t.Fatal(err)
	}
	next, _ := restarted.Append(ctx, Entry{Conversation: "g", Author: "you", Kind: "message", Ref: "m2", Body: map[string]string{"text": "later"}})
	if !mine.Clock.Less(next.Clock) || next.Prev != mine.ID {
		t.Fatalf("after a restart the clock and chain continue: %+v after %+v", next.Clock, mine.Clock)
	}
}

func TestBackfillUsesTheOriginalTimes(t *testing.T) {
	ctx := context.Background()
	log, _ := openLog(t)
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	r, err := log.Append(ctx, Entry{Conversation: "c", Author: "you", Kind: "message", Ref: "old", At: at, Body: map[string]string{"text": "yesterday"}})
	if err != nil || r.Clock.Wall != at.UnixMilli() {
		t.Fatalf("a backfilled record keeps its time: %v %+v", err, r.Clock)
	}
	same, _ := log.Append(ctx, Entry{Conversation: "c", Author: "you", Kind: "message", Ref: "old2", At: at, Body: map[string]string{"text": "same moment"}})
	if !r.Clock.Less(same.Clock) {
		t.Fatalf("records at the same moment still order: %+v %+v", r.Clock, same.Clock)
	}
}
