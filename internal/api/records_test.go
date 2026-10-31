package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"mace/internal/records"
)

func TestMessagesBecomeSignedRecords(t *testing.T) {
	tm := newTeam(t, "ari")
	ari := tm.agents["ari"]
	command(t, tm, ari.ID, "status", "")
	sent := tm.say(t, ari.ID, "hello")
	if sent.Record == "" {
		t.Fatal("a sent message should name its record")
	}
	list, err := tm.s.handler.records.Conversation(context.Background(), ari.ID)
	if err != nil || len(list) < 2 {
		t.Fatalf("records: %v %d", err, len(list))
	}
	mine := []records.Record{}
	for _, r := range list {
		if r.Node == tm.s.handler.records.Identity().ID {
			mine = append(mine, r)
		}
	}
	if err := records.VerifyChain(mine, tm.s.handler.records.Identity().Public); err != nil {
		t.Fatalf("this node's chain should verify: %v", err)
	}
	if list[0].Kind != "note" || list[1].ID != sent.Record {
		t.Fatalf("records follow the conversation: %s then %s", list[0].Kind, list[1].ID)
	}
}

func TestEarlierMessagesAreBackfilledOnce(t *testing.T) {
	tm := newTeam(t, "ari")
	h := tm.s.handler
	room := tm.agents["ari"].ID
	old := Message{ID: "old-1", Room: room, Sender: "you", Text: "from before records", At: time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC), Delivery: map[string]string{}}
	h.store.AppendMessage(context.Background(), room, old.ID, old)

	h.records = openRecords(h.store)
	list, _ := h.records.Conversation(context.Background(), room)
	if len(list) != 1 || list[0].Clock.Wall != old.At.UnixMilli() {
		t.Fatalf("the earlier message should become one record at its own time: %+v", list)
	}
	row, _ := h.store.MessageByID(context.Background(), old.ID)
	if !strings.Contains(string(row.Data), list[0].ID) {
		t.Fatal("the message should keep its record ID")
	}
	h.records = openRecords(h.store)
	if again, _ := h.records.Conversation(context.Background(), room); len(again) != 1 {
		t.Fatalf("a second start must not write duplicates, got %d", len(again))
	}
}
