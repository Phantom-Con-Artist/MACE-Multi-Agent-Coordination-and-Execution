package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mace/pkg/protocol"
)

// slowAgent installs a fake agy that keeps working until it is stopped.
func slowAgent(t *testing.T, s *Server) Session {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if e := os.WriteFile(filepath.Join(dir, "agy"), []byte("#!/bin/sh\nsleep 30\necho done\n"), 0700); e != nil {
		t.Fatal(e)
	}
	var a Session
	if call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": "agy"}, &a) != 201 {
		t.Fatal("session not created")
	}
	return a
}

func liveSessionProcesses(t *testing.T, s *Server, session string) int {
	t.Helper()
	list, _ := s.handler.supervisor.ListAgents(context.Background())
	n := 0
	for _, p := range list {
		if p.Labels["session"] == session && !p.Status.IsTerminal() {
			n++
		}
	}
	return n
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for", what)
}

func TestOwnerDisconnectStopsAgentsAndContinueResumes(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	a := slowAgent(t, s)

	if call(t, s, "POST", "/api/v1/presence", map[string]string{"device": "phone"}, nil) != 200 {
		t.Fatal("ping failed")
	}
	if call(t, s, "POST", "/api/v1/chats/"+a.ID+"/messages", map[string]string{"text": "build it"}, nil) != 201 {
		t.Fatal("send failed")
	}
	waitFor(t, "agent process", func() bool { return liveSessionProcesses(t, s, a.ID) == 1 })

	// Still present within the timeout and grace period: nothing stops.
	if s.handler.ownerAway(time.Now().Add(5 * time.Second)) {
		t.Fatal("guard fired while the device was still present")
	}
	if !s.handler.ownerAway(time.Now().Add(presenceTimeout + 16*time.Second)) {
		t.Fatal("guard should fire after timeout plus grace")
	}
	s.handler.stopWork(errOwnerAway, true)

	run := waitRun(t, s, a.ID, func(r Run) bool { return r.Status == "interrupted" })
	if !strings.Contains(run.Error, "Connection to your device was lost") {
		t.Fatalf("unexpected reason %q", run.Error)
	}
	waitFor(t, "process stop", func() bool { return liveSessionProcesses(t, s, a.ID) == 0 })
	var history []Message
	call(t, s, "GET", "/api/v1/chats/"+a.ID+"/messages", nil, &history)
	if last := history[len(history)-1]; last.Kind != "system" || last.Sender != "mace" {
		t.Fatalf("expected a visible system note, got %+v", last)
	}
	// The guard disarms after firing, so it does not fire again with nobody connected.
	if s.handler.ownerAway(time.Now().Add(time.Hour)) {
		t.Fatal("guard fired twice")
	}

	if call(t, s, "POST", "/api/v1/chats/"+a.ID+"/continue", nil, nil) != 201 {
		t.Fatal("continue failed")
	}
	waitRun(t, s, a.ID, func(r Run) bool { return r.Status == "running" })
	waitFor(t, "resumed process", func() bool { return liveSessionProcesses(t, s, a.ID) == 1 })
	call(t, s, "POST", "/api/v1/chats/"+a.ID+"/pause", nil, nil)
	run = waitRun(t, s, a.ID, func(r Run) bool { return r.Status != "running" })
	if run.Status != "paused" {
		t.Fatalf("an owner pause is a plain pause, got %s", run.Status)
	}
}

func TestGuardOffAndUnarmed(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	// Never pinged: work started through the API alone is not guarded.
	if s.handler.ownerAway(time.Now().Add(time.Hour)) {
		t.Fatal("unarmed guard fired")
	}
	t.Setenv("MACE_PRESENCE_GRACE_SECONDS", "off")
	call(t, s, "POST", "/api/v1/presence", map[string]string{"device": "phone"}, nil)
	if s.handler.ownerAway(time.Now().Add(time.Hour)) {
		t.Fatal("disabled guard fired")
	}
}

func TestPresenceLeaveWithZeroGraceAllowsReload(t *testing.T) {
	t.Setenv("MACE_PRESENCE_GRACE_SECONDS", "0")
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	call(t, s, "POST", "/api/v1/presence", map[string]string{"device": "phone"}, nil)
	call(t, s, "POST", "/api/v1/presence/leave", map[string]string{"device": "phone"}, nil)
	if s.handler.ownerAway(time.Now()) {
		t.Fatal("a reload must not count as leaving")
	}
	if !s.handler.ownerAway(time.Now().Add(reloadSlack + time.Second)) {
		t.Fatal("expected the guard to fire shortly after the last device left")
	}
}

func TestStopWorkLeavesPeerTurnsToTheirOrigin(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	start := func(origin string) string {
		p, e := sup.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"30"}, Labels: map[string]string{"session": "session_x", "origin": origin}})
		if e != nil {
			t.Fatal(e)
		}
		return p.ID
	}
	local, peer := start("local"), start("peer")
	app, _ := sup.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"30"}})
	defer sup.StopAgent(app.ID, "SIGKILL", time.Second)
	s.handler.stopWork(errOwnerAway, true)
	status := func(id string) bool { p, _ := sup.GetAgent(id); return p.Status.IsTerminal() }
	if !status(local) || status(peer) || status(app.ID) {
		t.Fatal("owner-away stop must end local agent turns only, never apps or peer turns")
	}
	s.handler.stopWork(errPausedAll, false)
	if !status(peer) || status(app.ID) {
		t.Fatal("pause everything ends peer turns too, but never desktop apps")
	}
}

func TestPauseAllInterruptsRuns(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	a := slowAgent(t, s)
	call(t, s, "POST", "/api/v1/chats/"+a.ID+"/messages", map[string]string{"text": "go"}, nil)
	waitFor(t, "agent process", func() bool { return liveSessionProcesses(t, s, a.ID) == 1 })
	var out map[string]any
	if call(t, s, "POST", "/api/v1/pause-all", nil, &out) != 200 || out["runs"].(float64) != 1 {
		t.Fatalf("unexpected pause-all result %v", out)
	}
	run := waitRun(t, s, a.ID, func(r Run) bool { return r.Status == "interrupted" })
	if !strings.Contains(run.Error, "Pause everything") {
		t.Fatalf("unexpected reason %q", run.Error)
	}
}

func TestDuplicateSendReturnsOriginal(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	t.Setenv("MACE_TEST_MODE", "1")
	var a Session
	call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": "diagnostic"}, &a)
	var first, second Message
	if call(t, s, "POST", "/api/v1/chats/"+a.ID+"/messages", map[string]string{"text": "once", "client_id": "c-1"}, &first) != 201 {
		t.Fatal("first send failed")
	}
	if call(t, s, "POST", "/api/v1/chats/"+a.ID+"/messages", map[string]string{"text": "once", "client_id": "c-1"}, &second) != 200 || second.ID != first.ID {
		t.Fatal("retry should return the original message")
	}
	waitRun(t, s, a.ID, func(r Run) bool { return r.Status != "running" })
	var history []json.RawMessage
	call(t, s, "GET", "/api/v1/chats/"+a.ID+"/messages", nil, &history)
	owner := 0
	for _, b := range history {
		var m Message
		json.Unmarshal(b, &m)
		if m.Sender == "you" {
			owner++
		}
	}
	if owner != 1 {
		t.Fatalf("expected one owner message, got %d", owner)
	}
}

func TestUnknownRemoteAgentIsUnavailable(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	if s.handler.available(context.Background(), "peer1::session_never_seen") {
		t.Fatal("an unreported remote agent must not be treated as available")
	}
}

func TestRestartMarksRunningWorkInterrupted(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer sup.Close()
	defer store.Close()
	store.PutDocument(context.Background(), "run", "room1", Run{ID: "room1", Status: "running", ActiveMember: "session_a"})
	_ = s
	h := NewAPIHandler(sup, store, protocol.NodeInfo{})
	var run Run
	h.store.Document(context.Background(), "run", "room1", &run)
	if run.Status != "interrupted" || run.ActiveMember != "" {
		t.Fatalf("expected interrupted after restart, got %+v", run)
	}
}
