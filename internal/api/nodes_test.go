package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// machine is one MACE node served over real HTTP, as it would be on a tailnet.
type machine struct {
	s  *Server
	ts *httptest.Server
}

func newMachine(t *testing.T, name string) *machine {
	t.Helper()
	s, sup, store := setupTestServer(t, "secret-"+name)
	t.Cleanup(func() { sup.Close(); store.Close() })
	ts := httptest.NewServer(s.Router())
	t.Cleanup(ts.Close)
	s.handler.advertise = ts.URL
	s.handler.nodeInfo.HostName = name
	return &machine{s: s, ts: ts}
}

func (m *machine) owner(t *testing.T, method, path string, body, out any) int {
	t.Helper()
	return callAs(t, m.s, "secret-"+m.s.handler.nodeInfo.HostName, method, path, body, out)
}

func (m *machine) invite(t *testing.T) string {
	t.Helper()
	var inv joinInvite
	if code := m.owner(t, "POST", "/api/v1/nodes/invite", map[string]any{}, &inv); code != 200 || inv.Code == "" || inv.URL != m.ts.URL {
		t.Fatalf("invite: %d %+v", code, inv)
	}
	return inv.Code
}

func (m *machine) nodes(t *testing.T) map[string]NodeView {
	t.Helper()
	var list []NodeView
	m.owner(t, "GET", "/api/v1/nodes", nil, &list)
	out := map[string]NodeView{}
	for _, n := range list {
		out[n.Name] = n
	}
	return out
}

func TestJoiningIsOneTimeAndMutual(t *testing.T) {
	laptop, pc := newMachine(t, "laptop"), newMachine(t, "pc")
	code := laptop.invite(t)
	if c := pc.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": code}, nil); c != 200 {
		t.Fatalf("join failed: %d", c)
	}
	if n := laptop.nodes(t); !n["pc"].Available || n["laptop"].Self != true {
		t.Fatalf("the laptop should list itself and the pc: %+v", n)
	}
	if n := pc.nodes(t); !n["laptop"].Available {
		t.Fatalf("the pc should list the laptop: %+v", n)
	}
	other := newMachine(t, "other")
	if c := other.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": code}, nil); c == 200 {
		t.Fatal("a join code must work only once")
	}
	if c := other.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": "AAAA-BBBB-CCCC-DDDD"}, nil); c == 200 {
		t.Fatal("an unknown code must be refused")
	}
}

func TestNewMachinesAreIntroducedToEveryone(t *testing.T) {
	laptop, pc, desk := newMachine(t, "laptop"), newMachine(t, "pc"), newMachine(t, "desk")
	pc.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": laptop.invite(t)}, nil)
	desk.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": laptop.invite(t)}, nil)
	// The laptop announced the desk to the pc, and told the desk about the pc.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pc.nodes(t)["desk"]; ok {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if _, ok := pc.nodes(t)["desk"]; !ok {
		t.Fatal("the pc should learn about the desk")
	}
	if _, ok := desk.nodes(t)["pc"]; !ok {
		t.Fatal("the desk should learn about the pc")
	}
}

func TestNodeRequestsMustBeSignedByAJoinedNode(t *testing.T) {
	laptop, pc := newMachine(t, "laptop"), newMachine(t, "pc")
	pc.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": laptop.invite(t)}, nil)

	self := laptop.s.handler.selfNode()
	var pong nodePong
	if e := pc.s.handler.nodeRequest(context.Background(), self, "GET", "/api/v1/node/ping", nil, &pong); e != nil || pong.Node.ID != self.ID {
		t.Fatalf("a joined node should be able to ping: %v", e)
	}
	stranger := newMachine(t, "stranger")
	if e := stranger.s.handler.nodeRequest(context.Background(), self, "GET", "/api/v1/node/ping", nil, &pong); e == nil {
		t.Fatal("a node that never joined must be refused")
	}
	res, _ := http.Get(laptop.ts.URL + "/api/v1/node/ping")
	if res.StatusCode != 401 {
		t.Fatalf("an unsigned request must be refused, got %d", res.StatusCode)
	}
}

func TestUnreachableNodesBecomeUnavailableAndRemovedOnesStayGone(t *testing.T) {
	laptop, pc := newMachine(t, "laptop"), newMachine(t, "pc")
	pc.owner(t, "POST", "/api/v1/nodes/join", map[string]string{"url": laptop.ts.URL, "code": laptop.invite(t)}, nil)
	pcID := pc.s.handler.selfNode().ID
	// The pc checks in with the laptop right after joining; an incoming check
	// counts as being alive, so let it finish before taking the pc offline.
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && !pc.nodes(t)["laptop"].Available; {
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	pc.ts.Close()
	laptop.s.handler.checkNodes(context.Background())
	if n := laptop.nodes(t)["pc"]; n.Available {
		t.Fatal("a machine that stopped answering should be unavailable")
	}
	if c := laptop.owner(t, "POST", "/api/v1/nodes/"+pcID+"/remove", map[string]any{}, nil); c != 200 {
		t.Fatalf("remove: %d", c)
	}
	// Gossip that still mentions the pc must not bring it back.
	laptop.s.handler.learnNodes(context.Background(), []Node{pc.s.handler.selfNode()}, "someone")
	if _, ok := laptop.nodes(t)["pc"]; ok {
		t.Fatal("a removed machine must stay removed")
	}
}
