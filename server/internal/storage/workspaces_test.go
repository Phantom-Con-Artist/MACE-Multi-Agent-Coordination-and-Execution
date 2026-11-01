package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionsAreIsolatedAndShareOnlyInfrastructure(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	// An installation from before sessions: one mace.db with everything.
	old, err := Open(filepath.Join(dir, "mace.db"))
	if err != nil {
		t.Fatal(err)
	}
	old.PutDocument(ctx, "node", "pc", map[string]string{"id": "pc"})
	old.PutDocument(ctx, "room", "r1", map[string]string{"id": "r1"})
	old.AppendMessage(ctx, "r1", "m1", map[string]string{"text": "old work"})
	old.Close()

	s, w, err := OpenWorkspaces(dir, "mace.db")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]string
	if w.ID != "default" || s.Document(ctx, "room", "r1", &doc) != nil || s.Document(ctx, "node", "pc", &doc) != nil {
		t.Fatalf("the old database should become the Default session with its links: %+v", w)
	}
	nw, err := s.AddWorkspace(ctx, "w2", "Launch")
	if err != nil || s.SetActiveWorkspace(ctx, nw.ID, time.Now()) != nil {
		t.Fatal(err)
	}
	s.Close()

	s, w, err = OpenWorkspaces(dir, "mace.db")
	if err != nil || w.ID != "w2" {
		t.Fatalf("the new session should open: %v %+v", err, w)
	}
	if s.Document(ctx, "room", "r1", &doc) == nil {
		t.Fatal("a new session must not see the old session's groups")
	}
	if msgs, _, _ := s.Messages(ctx, "r1", 0, 0, 10); len(msgs) != 0 {
		t.Fatal("a new session must not see the old session's messages")
	}
	if s.Document(ctx, "node", "pc", &doc) != nil {
		t.Fatal("linked machines are shared by every session")
	}
	if _, err := s.RemoveWorkspace(ctx, dir, "w2"); err == nil {
		t.Fatal("the active session cannot be removed")
	}
	rooms, err := s.RemoveWorkspace(ctx, dir, "default")
	if err != nil || len(rooms) != 1 || rooms[0] != "r1" {
		t.Fatalf("removing a session should report its groups: %v %v", rooms, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mace.db")); !os.IsNotExist(err) {
		t.Fatal("a removed session's database should be gone")
	}
	s.Close()
}
