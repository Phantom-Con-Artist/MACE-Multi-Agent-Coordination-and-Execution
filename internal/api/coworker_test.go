package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContinuousMembershipRolesAndGroupRemoval(t *testing.T) {
	t.Setenv("MACE_TEST_MODE", "1")
	t.Setenv("MACE_CONTINUOUS_GAP_SECONDS", "0.05")
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	var a, b, c Session
	for _, target := range []*Session{&a, &b, &c} {
		if call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": "diagnostic"}, target) != 201 {
			t.Fatal("session")
		}
	}
	var group Room
	call(t, s, "POST", "/api/v1/rooms", map[string]any{"name": "Coworkers", "members": []string{a.ID, b.ID}}, &group)
	if len(group.Roles) != 0 {
		t.Fatal("roles should be unassigned by default")
	}
	call(t, s, "POST", "/api/v1/chats/"+group.ID+"/policy", Policy{Mode: "continuous"}, nil)
	if call(t, s, "POST", "/api/v1/chats/"+group.ID+"/messages", map[string]string{"text": "keep going on the plan"}, nil) != 201 {
		t.Fatal("the owner's message should start the conversation")
	}
	t.Cleanup(func() {
		if t.Failed() {
			rows, _, _ := store.Messages(context.Background(), group.ID, 0, 0, 100)
			for _, r := range rows {
				t.Log(string(r.Data))
			}
		}
	})
	before := waitRun(t, s, group.ID, func(r Run) bool { return r.Status == "running" && r.Replies >= 1 })
	if call(t, s, "POST", "/api/v1/rooms/"+group.ID+"/members/"+b.ID+"/remove", map[string]any{}, &group) != 200 {
		t.Fatal("remove member")
	}
	if len(group.Members) != 1 || group.Members[0] != a.ID {
		t.Fatal(group)
	}
	waitRun(t, s, group.ID, func(r Run) bool { return r.Status == "running" && r.Replies >= before.Replies+2 })
	rolePath := "/api/v1/rooms/" + group.ID + "/members/" + a.ID + "/role"
	if call(t, s, "POST", rolePath, map[string]string{"role": "Review the implementation and ask for evidence"}, &group) != 200 || group.Roles[a.ID] == "" {
		t.Fatal("assign role")
	}
	var cleared Room
	if call(t, s, "POST", rolePath, map[string]string{"role": ""}, &cleared) != 200 || len(cleared.Roles) != 0 {
		t.Fatal("clear role")
	}
	if call(t, s, "POST", "/api/v1/rooms/"+group.ID+"/members", map[string]string{"member": c.ID}, &group) != 200 {
		t.Fatal("add coworker during a run")
	}
	waitRun(t, s, group.ID, func(r Run) bool { return r.Replies >= before.Replies+4 })
	messages, _, _ := store.Messages(context.Background(), group.ID, 0, 0, 1000)
	foundC := false
	for _, raw := range messages {
		var m Message
		json.Unmarshal(raw.Data, &m)
		if m.Sender == c.ID {
			foundC = true
		}
	}
	if !foundC {
		t.Fatal("new member never got a turn")
	}
	if call(t, s, "POST", "/api/v1/sessions/"+c.ID+"/control", map[string]string{"action": "stop"}, nil) != 200 {
		t.Fatal("stop coworker")
	}
	current := waitRun(t, s, group.ID, func(r Run) bool { return r.Status == "running" })
	waitRun(t, s, group.ID, func(r Run) bool { return r.Status == "running" && r.Replies > current.Replies })
	if call(t, s, "POST", "/api/v1/rooms/"+group.ID+"/remove", map[string]any{}, nil) != 200 {
		t.Fatal("remove group")
	}
	waitRun(t, s, group.ID, func(r Run) bool { return r.Status == "paused" })
	var rooms []Room
	call(t, s, "GET", "/api/v1/rooms", nil, &rooms)
	if len(rooms) != 0 {
		t.Fatal("removed group is still visible")
	}
	var runs []Run
	call(t, s, "GET", "/api/v1/runs", nil, &runs)
	for _, r := range runs {
		if r.ID == group.ID {
			t.Fatal("removed group is still in working conversations")
		}
	}
	if call(t, s, "POST", "/api/v1/chats/"+group.ID+"/messages", map[string]string{"text": "should not deliver"}, nil) != 410 {
		t.Fatal("removed group still accepts work")
	}
	var saved Room
	store.Document(context.Background(), "room", group.ID, &saved)
	if !saved.Removed {
		t.Fatal("removal not persisted")
	}
	history, _, _ := store.Messages(context.Background(), group.ID, 0, 0, 1000)
	if len(history) < 2 {
		t.Fatal("group history was deleted")
	}
}

func TestCoworkerContextAndNativeBootstrap(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	a := Session{ID: "session_a", Name: "Ari", Provider: "codex", Directory: t.TempDir(), Status: "ready"}
	b := Session{ID: "session_b", Name: "Bo", Provider: "codex", Directory: a.Directory, Status: "ready"}
	store.PutDocument(context.Background(), "session", a.ID, a)
	store.PutDocument(context.Background(), "session", b.ID, b)
	room := Room{ID: "team", Name: "Project team", Members: []string{a.ID, b.ID}, Roles: map[string]string{a.ID: "Review accessibility"}}
	store.PutDocument(context.Background(), "room", room.ID, room)
	s.handler.ensureHandles(context.Background())
	task := Message{ID: newID(), Room: room.ID, Sender: "you", Text: strings.Repeat("earlier context ", 9000) + "latest task stays available"}
	s.handler.postMessage(context.Background(), &task)
	prompts, last, _ := s.handler.buildPrompts(context.Background(), &turnRequest{Member: a.ID, Room: room.ID}, room.Members, &room, Policy{Mode: "continuous"})
	prompt := prompts.Full
	if prompts.Delta != "" || last != task.Seq {
		t.Fatal("a first turn has no cursor yet, so only the full form applies")
	}
	for _, term := range []string{"MACE GROUP", "Ari", "Bo", "@codex", "Review accessibility", "Unassigned", "coworkers", "without waiting", "latest task stays available"} {
		if !strings.Contains(prompt, term) {
			t.Fatal("missing coworker context", term)
		}
	}
	dir := t.TempDir()
	capture := filepath.Join(dir, "prompt.txt")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Children never receive MACE_* variables, so the capture path uses another name.
	t.Setenv("TEST_PROMPT_LOG", capture)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$TEST_PROMPT_LOG\"\nprintf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"coworker response\"}}'\n"
	if e := os.WriteFile(filepath.Join(dir, "codex"), []byte(script), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := s.handler.turn(context.Background(), a.ID, prompts, "local", nil); e != nil {
		t.Fatal(e)
	}
	sent, e := os.ReadFile(capture)
	if e != nil {
		t.Fatal(e)
	}
	for _, term := range []string{"Multi Agent Coordination and Execution", "AI coworker", "MACE AWARENESS", "Working folder", "Group \"Project team\"", "MACE GROUP", "Review accessibility", "latest task stays available", "[PASS]"} {
		if !strings.Contains(string(sent), term) {
			t.Fatal("native CLI did not receive", term)
		}
	}
}
