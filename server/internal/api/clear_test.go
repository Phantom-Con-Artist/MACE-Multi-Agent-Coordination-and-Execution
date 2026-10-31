package api

import (
	"context"
	"encoding/json"
	"testing"
)

func TestClearingAChatDeletesItsMessagesOnly(t *testing.T) {
	s, sup, store := setupTestServer(t, "")
	defer store.Close()
	defer sup.Close()
	ctx := context.Background()
	for _, id := range []string{"session_one", "session_two"} {
		store.PutDocument(ctx, "session", id, Session{ID: id, Name: id, Status: "ready"})
	}
	for i, room := range []string{"session_one", "session_one", "session_two"} {
		id := []string{"a", "b", "c"}[i]
		if _, err := store.AppendMessage(ctx, room, id, Message{ID: id, Room: room, Sender: "you", Text: id}); err != nil {
			t.Fatal(err)
		}
	}
	w := request(s, "POST", "/api/v1/chats/session_one/clear", "{}")
	if w.Code != 200 {
		t.Fatalf("clear answered %d: %s", w.Code, w.Body.String())
	}
	var out struct{ Cleared int }
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Cleared != 2 {
		t.Fatalf("cleared %d messages, want 2", out.Cleared)
	}
	left := func(room string) int {
		rows, _, _ := store.Messages(ctx, room, 0, 0, 0)
		return len(rows)
	}
	if left("session_one") != 0 || left("session_two") != 1 {
		t.Fatalf("after clearing one chat: one has %d, two has %d", left("session_one"), left("session_two"))
	}
	if request(s, "POST", "/api/v1/chats/session_missing/clear", "{}").Code != 404 {
		t.Fatal("clearing a chat that does not exist should be a 404")
	}
}
