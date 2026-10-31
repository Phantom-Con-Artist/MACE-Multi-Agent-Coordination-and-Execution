package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hubAndWorker joins a worker to a hub and starts one scripted agent on the
// worker, returning the agent as the hub sees it.
func hubAndWorker(t *testing.T) (hub, worker *machine, agent string, dir string) {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	os.WriteFile(filepath.Join(bin, "agy"), []byte(scriptedAgy), 0700)
	t.Setenv("TIMELINE", filepath.Join(bin, "timeline"))
	hub, worker = newMachine(t, "hub"), newMachine(t, "worker")
	joinAll(t, hub, worker)
	dir = filepath.Join(t.TempDir(), "muse")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "reply.txt"), []byte("hello from the worker"), 0600)
	var s Session
	if c := worker.owner(t, "POST", "/api/v1/sessions", map[string]string{"provider": "agy", "working_dir": dir}, &s); c != 201 {
		t.Fatalf("create agent on the worker: %d", c)
	}
	return hub, worker, worker.s.handler.selfNode().ID + "::" + s.ID, dir
}

func TestJoinedMachinesShareAgentsTurnsAndHistory(t *testing.T) {
	hub, worker, agent, _ := hubAndWorker(t)
	node, local, _ := strings.Cut(agent, "::")

	var peers []Peer
	hub.owner(t, "GET", "/api/v1/peers", nil, &peers)
	if len(peers) != 1 || peers[0].ID != node || peers[0].Node != node {
		t.Fatalf("the joined worker should be a peer with no token: %+v", peers)
	}
	var sessions []Session
	if c := hub.owner(t, "GET", "/api/v1/peers/"+node+"/sessions", nil, &sessions); c != 200 || len(sessions) != 1 || sessions[0].ID != agent {
		t.Fatalf("the hub should list the worker's agent over a signed request: %d %+v", c, sessions)
	}

	// The owner talks to the worker's agent from the hub.
	if c := hub.owner(t, "POST", "/api/v1/chats/"+agent+"/messages", map[string]string{"text": "hi from the phone"}, nil); c != 201 {
		t.Fatalf("send: %d", c)
	}
	settled(t, hub.s, agent)
	var hubSide []Message
	hub.owner(t, "GET", "/api/v1/chats/"+agent+"/messages", nil, &hubSide)
	if len(hubSide) != 2 || hubSide[1].Sender != agent || !strings.Contains(hubSide[1].Text, "hello from the worker") {
		t.Fatalf("the worker's agent should answer through the hub: %+v", hubSide)
	}

	// The worker syncs the hub's records and shows that side of the chat.
	worker.syncNow(t)
	var workerSide []Message
	worker.owner(t, "GET", "/api/v1/chats/"+local+"/messages", nil, &workerSide)
	if len(workerSide) != 2 || workerSide[0].Origin != "hub" || workerSide[1].Sender != local {
		t.Fatalf("the worker should show the chat held on the hub, as its own agent: %+v", workerSide)
	}

	// And the owner's direct chat on the worker shows on the hub.
	worker.owner(t, "POST", "/api/v1/chats/"+local+"/messages", map[string]string{"text": "hi from the pc"}, nil)
	settled(t, worker.s, local)
	hub.syncNow(t)
	hub.owner(t, "GET", "/api/v1/chats/"+agent+"/messages", nil, &hubSide)
	found := false
	for _, m := range hubSide {
		found = found || (m.Text == "hi from the pc" && m.Origin == "worker")
	}
	if !found || len(hubSide) != 4 {
		t.Fatalf("the hub should merge the worker's side of the chat: %+v", hubSide)
	}
}

func TestFilesMoveBetweenMachinesAsVerifiedChunks(t *testing.T) {
	hub, worker, agent, _ := hubAndWorker(t)
	node, _, _ := strings.Cut(agent, "::")
	src := filepath.Join(t.TempDir(), "design.bin")
	data := bytes.Repeat([]byte("0123456789abcdef"), 600_000) // about 9.6 MB, three chunks
	os.WriteFile(src, data, 0600)

	dest, err := hub.s.handler.transfer(context.Background(), node, strings.TrimPrefix(src, "/"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dest); !bytes.Equal(got, data) {
		t.Fatalf("the worker should hold an identical copy at %s", dest)
	}
	// The worker now holds every chunk, so sending the same content again
	// fetches only its manifest.
	sent, _ := hub.s.handler.blobStore()
	m, err := sent.Add(bytes.NewReader(data), "design.bin", "")
	if err != nil || len(m.Chunks) != 3 {
		t.Fatalf("expected three chunks: %v", err)
	}
	received, _ := worker.s.handler.blobStore()
	if missing := received.Missing(m); len(missing) != 0 {
		t.Fatalf("the worker should already hold every chunk: %v", missing)
	}
	if _, err := hub.s.handler.transfer(context.Background(), node, strings.TrimPrefix(src, "/")); err != nil {
		t.Fatal(err)
	}
}

func TestSignedRequestsReachOnlyTheOwnerPathsMeantForMachines(t *testing.T) {
	hub, worker, agent, _ := hubAndWorker(t)
	node, _, _ := strings.Cut(agent, "::")
	var p Peer
	hub.s.handler.peerDoc(context.Background(), node, &p)
	if err := hub.s.handler.peerRequest(context.Background(), p, "/sessions", nil, &[]Session{}); err != nil {
		t.Fatalf("listing agents is a machine path: %v", err)
	}
	err := hub.s.handler.peerRequest(context.Background(), p, "/rooms", map[string]any{"name": "x"}, &Room{})
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "older MACE") {
		t.Fatalf("creating groups on another machine is the owner's alone: %v", err)
	}
	stranger := newMachine(t, "stranger")
	err = stranger.s.handler.signedRequest(context.Background(), "worker", worker.ts.URL, "GET", "/api/v1/sessions", nil, &[]Session{}, 0)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a machine that never joined must be refused: %v", err)
	}
}

func TestTheHubStartsAgentsOnAWorker(t *testing.T) {
	hub, worker, _, dir := hubAndWorker(t)
	node := worker.s.handler.selfNode().ID
	var s Session
	if c := hub.owner(t, "POST", "/api/v1/peers/"+node+"/sessions", map[string]string{"provider": "agy", "working_dir": dir}, &s); c != 201 || !strings.HasPrefix(s.ID, node+"::") {
		t.Fatalf("start on the worker: %d %+v", c, s)
	}
	var mine []Session
	worker.owner(t, "GET", "/api/v1/sessions", nil, &mine)
	if len(mine) != 2 {
		t.Fatalf("the worker should run the new agent itself: %+v", mine)
	}
	var cat struct {
		Providers []Provider `json:"providers"`
	}
	if c := hub.owner(t, "GET", "/api/v1/peers/"+node+"/catalog", nil, &cat); c != 200 || len(cat.Providers) == 0 {
		t.Fatalf("the hub should see the worker's CLIs: %d", c)
	}
}

func TestTheHubBrowsesAWorkersFilesAndApps(t *testing.T) {
	hub, worker, _, dir := hubAndWorker(t)
	node := worker.s.handler.selfNode().ID
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello from the pc"), 0600)
	get := func(path string, header map[string]string) (int, string, http.Header) {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Authorization", "Bearer secret-hub")
		for k, v := range header {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		hub.s.Router().ServeHTTP(w, r)
		return w.Code, w.Body.String(), w.Header()
	}
	base := "/api/v1/machines/" + node
	code, body, _ := get(base+"/files?path="+url.QueryEscape(dir), nil)
	if code != 200 || !strings.Contains(body, "notes.txt") {
		t.Fatalf("the hub should list the worker's folder: %d %s", code, body)
	}
	code, body, h := get(base+"/files?path="+url.QueryEscape(filepath.Join(dir, "notes.txt")), map[string]string{"Range": "bytes=0-4"})
	if code != 206 || body != "hello" || h.Get("Content-Security-Policy") != "sandbox" {
		t.Fatalf("previews should stream with ranges and stay sandboxed: %d %q %v", code, body, h)
	}
	if code, _, _ := get(base+"/apps", nil); code != 200 {
		t.Fatalf("the hub should list the worker's apps: %d", code)
	}
	if code, _, _ := get(base+"/rooms", nil); code != 404 {
		t.Fatalf("only machine paths are forwarded: %d", code)
	}
}

func TestWorkerAgentsShareFilesAndFoldersIntoHubGroups(t *testing.T) {
	hub, _, agent, dir := hubAndWorker(t)
	os.MkdirAll(filepath.Join(dir, "draft", "css"), 0700)
	os.WriteFile(filepath.Join(dir, "draft", "index.html"), []byte("<h1>draft</h1>"), 0600)
	os.WriteFile(filepath.Join(dir, "draft", "css", "a.css"), []byte("h1{}"), 0600)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("research notes"), 0600)
	os.WriteFile(filepath.Join(dir, "reply.txt"), []byte("here [share: draft] and [share: notes.md]"), 0600)
	node, _, _ := strings.Cut(agent, "::")
	hub.owner(t, "GET", "/api/v1/peers/"+node+"/sessions", nil, &[]Session{}) // as the app does
	var room Room
	if c := hub.owner(t, "POST", "/api/v1/rooms", map[string]any{"name": "Team", "members": []string{agent}}, &room); c != 201 {
		t.Fatalf("group: %d", c)
	}
	hub.owner(t, "POST", "/api/v1/chats/"+room.ID+"/messages", map[string]string{"text": "send your drafts"}, nil)
	settled(t, hub.s, room.ID)
	var msgs []Message
	hub.owner(t, "GET", "/api/v1/chats/"+room.ID+"/messages", nil, &msgs)
	got := map[string]string{}
	for _, m := range msgs {
		if m.File != "" {
			got[filepath.Base(strings.TrimSuffix(m.File, "/"))] = m.File
		}
		if strings.Contains(m.Text, "could not share") {
			t.Fatalf("both shares should arrive: %q", m.Text)
		}
	}
	folder := got[firstKey(got, "draft")]
	if !strings.HasSuffix(folder, "/") {
		t.Fatalf("the folder should be attached as a folder: %v", got)
	}
	if b, _ := os.ReadFile("/" + folder + "css/a.css"); string(b) != "h1{}" {
		t.Fatalf("the hub should hold a copy of the whole folder at %s", folder)
	}
	if b, _ := os.ReadFile("/" + got[firstKey(got, "notes.md")]); string(b) != "research notes" {
		t.Fatalf("the hub should hold the file: %v", got)
	}
}

// firstKey finds the attachment whose name ends with suffix (names carry a
// unique prefix).
func firstKey(m map[string]string, suffix string) string {
	for k := range m {
		if strings.HasSuffix(k, suffix) {
			return k
		}
	}
	return ""
}

func TestTheHubSendsWholeFoldersToWorkers(t *testing.T) {
	hub, _, agent, _ := hubAndWorker(t)
	node, _, _ := strings.Cut(agent, "::")
	src := filepath.Join(t.TempDir(), "site")
	os.MkdirAll(filepath.Join(src, "img"), 0700)
	os.WriteFile(filepath.Join(src, "index.html"), []byte("<p>ok</p>"), 0600)
	os.WriteFile(filepath.Join(src, "img", "logo.svg"), []byte("<svg/>"), 0600)
	dest, err := hub.s.handler.transfer(context.Background(), node, strings.TrimPrefix(src, "/")+"/")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "img", "logo.svg")); string(b) != "<svg/>" {
		t.Fatalf("the worker should hold the folder at %s", dest)
	}
}

func TestOutdatedMachinesAreFlaggedUntilTheyUpdate(t *testing.T) {
	hub, worker, _, _ := hubAndWorker(t)
	ctx := context.Background()
	id := worker.s.handler.selfNode().ID
	var n Node
	hub.s.handler.store.Document(ctx, "node", id, &n)
	if n.Protocol != nodeProtocol {
		t.Fatalf("a joining machine should report its version: %d", n.Protocol)
	}
	n.Protocol = 1 // as if the worker ran an older MACE
	hub.s.handler.store.PutDocument(ctx, "node", id, n)
	if !hub.nodes(t)["worker"].Outdated {
		t.Fatal("an older machine should be flagged")
	}
	hub.s.handler.checkNode(ctx, n, 0) // the next check sees the update
	if hub.nodes(t)["worker"].Outdated {
		t.Fatal("the flag should clear once the machine reports the current version")
	}
}
