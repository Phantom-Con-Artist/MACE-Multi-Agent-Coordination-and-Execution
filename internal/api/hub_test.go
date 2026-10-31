package api

import (
	"bytes"
	"context"
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
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("creating groups on another machine is the owner's alone: %v", err)
	}
	stranger := newMachine(t, "stranger")
	err = stranger.s.handler.signedRequest(context.Background(), "worker", worker.ts.URL, "GET", "/api/v1/sessions", nil, &[]Session{}, 0)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("a machine that never joined must be refused: %v", err)
	}
}
