package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func call(t *testing.T, s *Server, method, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code >= 400 {
		t.Log(w.Body.String())
	}
	if out != nil && w.Code < 400 {
		if e := json.Unmarshal(w.Body.Bytes(), out); e != nil {
			t.Fatal(e)
		}
	}
	return w.Code
}
func waitRun(t *testing.T, s *Server, id string, predicate func(Run) bool) Run {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		var run Run
		if s.handler.store.Document(context.Background(), "run", id, &run) == nil && predicate(run) {
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("run did not reach expected state")
	return Run{}
}
func TestCollaborationModesAndLifecycle(t *testing.T) {
	t.Setenv("MACE_TEST_MODE", "1")
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	var a, b Session
	if call(t, s, "POST", "/api/v1/sessions", map[string]any{"provider": "diagnostic"}, &a) != 201 {
		t.Fatal("create session")
	}
	if a.Directory != "/" {
		t.Fatal("default directory must be root")
	}
	call(t, s, "POST", "/api/v1/sessions", map[string]any{"provider": "diagnostic", "working_dir": t.TempDir()}, &b)
	var room Room
	call(t, s, "POST", "/api/v1/rooms", map[string]any{"name": "team", "members": []string{a.ID, b.ID}}, &room)
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "reply", Limit: 3}, nil)
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/messages", map[string]any{"text": "work together"}, nil)
	run := waitRun(t, s, room.ID, func(r Run) bool { return r.Status == "limit_reached" })
	if run.Replies != 3 {
		t.Fatal(run)
	}
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "goal", Limit: 5, Goal: "finish diagnostic"}, nil)
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/messages", map[string]any{"text": "complete the goal"}, nil)
	run = waitRun(t, s, room.ID, func(r Run) bool { return r.Status == "goal_reported_complete" })
	if run.Replies != 1 {
		t.Fatal(run)
	}
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/policy", Policy{Mode: "continuous"}, nil)
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/messages", map[string]any{"text": "continue"}, nil)
	waitRun(t, s, room.ID, func(r Run) bool { return r.Status == "running" && r.Replies >= 1 })
	call(t, s, "POST", "/api/v1/chats/"+room.ID+"/pause", map[string]any{}, nil)
	waitRun(t, s, room.ID, func(r Run) bool { return r.Status == "paused" })
	for _, action := range []string{"stop", "resume", "terminate", "resume", "remove"} {
		var out Session
		if call(t, s, "POST", "/api/v1/sessions/"+a.ID+"/control", map[string]string{"action": action}, &out) != 200 {
			t.Fatal(action)
		}
	}
	store.Document(context.Background(), "room", room.ID, &room)
	if len(room.Members) != 1 || room.Members[0] != b.ID {
		t.Fatal("removed agent remains in group")
	}
	raw, _ := store.Documents(context.Background(), "message:"+room.ID)
	if len(raw) < 8 {
		t.Fatal("missing conversation history")
	}
}

func TestPeerCollaborationAndFileTransfer(t *testing.T) {
	t.Setenv("MACE_TEST_MODE", "1")
	local, ls, lstore := setupTestServer(t, "secret")
	remote, rs, rstore := setupTestServer(t, "secret")
	defer lstore.Close()
	defer rstore.Close()
	defer ls.Close()
	defer rs.Close()
	local.handler.fileRoot = t.TempDir()
	remote.handler.fileRoot = t.TempDir()
	if e := os.WriteFile(filepath.Join(local.handler.fileRoot, "shared.txt"), []byte("a shared artifact"), 0600); e != nil {
		t.Fatal(e)
	}
	host := httptest.NewServer(remote.Router())
	defer host.Close()
	var p Peer
	if call(t, local, "POST", "/api/v1/peers", Peer{Name: "second host", URL: host.URL, Token: "secret"}, &p) != 201 {
		t.Fatal("connect peer")
	}
	if p.Token != "" {
		t.Fatal("token exposed")
	}
	var peerList []Peer
	call(t, local, "GET", "/api/v1/peers", nil, &peerList)
	if len(peerList) != 1 || peerList[0].Token != "" {
		t.Fatal("credentials leaked")
	}
	var a Session
	call(t, remote, "POST", "/api/v1/sessions", map[string]string{"provider": "diagnostic"}, &a)
	var list []Session
	call(t, local, "GET", "/api/v1/peers/"+p.ID+"/sessions", nil, &list)
	if len(list) != 1 {
		t.Fatal("remote agents missing")
	}
	var transfer map[string]string
	if call(t, local, "POST", "/api/v1/transfers", map[string]string{"peer": p.ID, "path": "shared.txt"}, &transfer) != 201 {
		t.Fatal("transfer failed")
	}
	data, e := os.ReadFile(transfer["path"])
	if e != nil || string(data) != "a shared artifact" {
		t.Fatal("remote data mismatch", e)
	}
	id := p.ID + "::" + a.ID
	id = strings.ReplaceAll(id, ":", "%3A")
	call(t, local, "POST", "/api/v1/chats/"+id+"/messages", map[string]string{"text": "inspect the artifact", "file": "shared.txt"}, nil)
	run := waitRun(t, local, strings.ReplaceAll(id, "%3A", ":"), func(r Run) bool { return r.Status != "running" })
	if run.Status != "limit_reached" || run.Replies != 1 {
		t.Fatal(run)
	}
	if call(t, local, "POST", "/api/v1/sessions/"+id+"/control", map[string]string{"action": "terminate"}, nil) != 200 {
		t.Fatal("remote lifecycle failed")
	}
	var out Session
	rstore.Document(context.Background(), "session", a.ID, &out)
	if out.Status != "terminated" {
		t.Fatal(out)
	}
	req, _ := http.NewRequest("GET", host.URL+"/api/v1/sessions", nil)
	res, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatal("remote API must require token")
	}
}
