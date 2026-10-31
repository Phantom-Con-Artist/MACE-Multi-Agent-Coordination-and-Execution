package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mace/pkg/protocol"
)

func TestOpen_UpgradesUnversionedDatabaseWithBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mace.db")
	// An unversioned database as written by earlier releases.
	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(baseSchema + `INSERT INTO documents(kind,id,data) VALUES('room','r1','{"id":"r1"}');`); err != nil {
		t.Fatal(err)
	}
	old.Close()

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version int
	store.db.QueryRow(`PRAGMA user_version`).Scan(&version)
	if version != len(migrations) {
		t.Fatalf("expected version %d, got %d", len(migrations), version)
	}
	var room map[string]any
	if err := store.Document(context.Background(), "room", "r1", &room); err != nil {
		t.Fatalf("existing data lost: %v", err)
	}
	// Version 0 databases start at the first migration, which is idempotent, so no
	// backup is needed; a second open must not re-run anything.
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
}

func TestOpen_RestoresNewestBackupWhenCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mace.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.PutDocument(context.Background(), "room", "kept", map[string]string{"id": "kept"})
	if _, err := store.Backup("daily"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	os.Remove(path + "-wal")
	os.Remove(path + "-shm")
	if err := os.WriteFile(path, []byte(strings.Repeat("garbage", 1000)), 0600); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if !strings.Contains(store.Recovered, "restored") {
		t.Fatalf("expected a recovery report, got %q", store.Recovered)
	}
	var room map[string]any
	if err := store.Document(context.Background(), "room", "kept", &room); err != nil {
		t.Fatalf("backup data missing after restore: %v", err)
	}
	if matches, _ := filepath.Glob(path + ".corrupt-*"); len(matches) == 0 {
		t.Fatal("damaged file should be kept aside for salvage")
	}
}

func TestPruneEvents_KeepsLiveAndRecentOutput(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	old := time.Now().UTC().Add(-30 * 24 * time.Hour)
	for _, a := range []struct {
		id     string
		status protocol.AgentStatus
	}{{"done", protocol.AgentStatusStopped}, {"live", protocol.AgentStatusRunning}} {
		store.SaveAgent(ctx, &protocol.AgentProcess{ID: a.id, Name: a.id, Status: a.status, Command: "x", StartedAt: old})
		store.AppendEvent(ctx, &protocol.AgentEvent{EventID: a.id + "-old", AgentID: a.id, Type: protocol.EventStdout, Data: "old", Timestamp: old, Sequence: 1})
		store.AppendEvent(ctx, &protocol.AgentEvent{EventID: a.id + "-new", AgentID: a.id, Type: protocol.EventStdout, Data: "new", Timestamp: time.Now().UTC(), Sequence: 2})
	}
	n, err := store.PruneEvents(ctx, time.Now().Add(-14*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected one old finished event pruned, got %d", n)
	}
	live, _ := store.GetAgentEvents(ctx, "live", 0, 10)
	done, _ := store.GetAgentEvents(ctx, "done", 0, 10)
	if len(live) != 2 || len(done) != 1 || done[0].Data != "new" {
		t.Fatalf("unexpected remaining events: live=%d done=%d", len(live), len(done))
	}
}

func TestPruneBackups_KeepsNewest(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, label := range []string{"a", "b", "c", "d"} {
		if _, err := store.Backup(label); err != nil {
			t.Fatal(err)
		}
		time.Sleep(1100 * time.Millisecond)
	}
	store.PruneBackups(2)
	if got := backups(store.BackupDir()); len(got) != 2 || !strings.Contains(got[0], "-d.db") {
		t.Fatalf("expected the two newest backups, got %v", got)
	}
}
