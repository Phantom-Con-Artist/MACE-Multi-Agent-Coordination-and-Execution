package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

// onTailnet gives machines a shared view of the owner's tailnet in which each
// is a device of the given user, reachable at its test address.
func onTailnet(owner string, all ...*machine) {
	for _, m := range all {
		self := m
		self.s.handler.tail.source = func() (tailnetView, error) {
			v := tailnetView{User: "u-me", Login: "me@example.com"}
			for _, other := range all {
				if other != self {
					v.Peers = append(v.Peers, tailnetPeer{Host: other.s.handler.nodeInfo.HostName, IPs: []string{"127.0.0.1"}, User: owner, Online: true, URLs: []string{other.ts.URL}})
				}
			}
			return v, nil
		}
	}
}

func TestTailscaleStatusNamesTheOwnerAndTheirDevices(t *testing.T) {
	v, err := parseTailscaleStatus([]byte(`{"BackendState":"Running","Self":{"UserID":7},
		"User":{"7":{"LoginName":"me@github"},"9":{"LoginName":"friend@github"}},
		"Peer":{"a":{"HostName":"pc","DNSName":"pc.tail.ts.net.","TailscaleIPs":["100.1.1.1"],"UserID":7,"Online":true},
		        "b":{"HostName":"friend","TailscaleIPs":["100.2.2.2"],"UserID":9,"Online":true},
		        "c":{"HostName":"server","TailscaleIPs":["100.3.3.3"],"UserID":7,"Tags":["tag:ci"]}}}`))
	if err != nil || v.Login != "me@github" || len(v.Peers) != 3 {
		t.Fatalf("%+v %v", v, err)
	}
	owned := map[string]bool{}
	for _, p := range v.Peers {
		owned[p.Host] = v.owned(p)
	}
	if !owned["pc"] || owned["friend"] || owned["server"] {
		t.Fatalf("only the owner's untagged devices count: %v", owned)
	}
}

func TestMachinesOnTheOwnersTailnetJoinWithoutCodes(t *testing.T) {
	hub, worker := newMachine(t, "hub"), newMachine(t, "worker")
	onTailnet("u-me", hub, worker)
	// The worker comes online after the hub and finds it by itself.
	worker.s.handler.discoverMachines(context.Background())
	if n := hub.nodes(t); !n["worker"].Available {
		t.Fatalf("the hub should know the worker: %+v", n)
	}
	if n := worker.nodes(t); !n["hub"].Available {
		t.Fatalf("the worker should know the hub: %+v", n)
	}
	// A machine the owner removed is not brought back by discovery.
	hub.owner(t, "POST", "/api/v1/nodes/"+worker.s.handler.selfNode().ID+"/remove", map[string]any{}, nil)
	worker.s.handler.store.DeleteDocument(context.Background(), "node", hub.s.handler.selfNode().ID)
	worker.s.handler.tail.probed = nil
	worker.s.handler.discoverMachines(context.Background())
	if _, ok := hub.nodes(t)["worker"]; ok {
		t.Fatal("a removed machine must be joined again with a code")
	}
}

func TestDevicesOfOtherAccountsDoNotJoin(t *testing.T) {
	hub, stranger := newMachine(t, "hub"), newMachine(t, "stranger")
	onTailnet("u-someone-else", hub, stranger)
	stranger.s.handler.discoverMachines(context.Background())
	if _, ok := hub.nodes(t)["stranger"]; ok {
		t.Fatal("another account's device must not join")
	}
	// Asking directly is refused too.
	self := stranger.s.handler.selfNode()
	err := postJSON(context.Background(), hub.ts.URL+"/api/v1/nodes/autojoin", map[string]any{"node": self, "proof": stranger.s.handler.records.Identity().Sign(autojoinProof(hub.s.handler.selfNode().ID, self.ID))}, &nodePong{})
	if err == nil {
		t.Fatal("the hub should refuse a device of another account")
	}
}

func TestPhonesOnTheOwnersAccountSignInThroughTailscaleServe(t *testing.T) {
	m := newMachine(t, "hub")
	onTailnet("u-me", m)
	session := func(login string) (bool, bool) {
		r := httptest.NewRequest("GET", "/api/session", nil)
		r.RemoteAddr = "127.0.0.1:50000" // tailscale serve forwards from this machine
		if login != "" {
			r.Header.Set("Tailscale-User-Login", login)
		}
		w := httptest.NewRecorder()
		m.s.Router().ServeHTTP(w, r)
		var out map[string]bool
		json.Unmarshal(w.Body.Bytes(), &out)
		return out["connected"], len(w.Result().Cookies()) > 0
	}
	if ok, cookie := session("ME@example.com"); !ok || !cookie {
		t.Fatalf("the owner's phone should be signed in: %v %v", ok, cookie)
	}
	if ok, _ := session("friend@example.com"); ok {
		t.Fatal("another user's device must not be signed in")
	}
	if ok, _ := session(""); ok {
		t.Fatal("a local request without Tailscale's identity must not be signed in")
	}
	t.Setenv("MACE_TAILSCALE_TRUST", "off")
	m.s.handler.tail.at = time.Time{}
	if ok, _ := session("me@example.com"); ok {
		t.Fatal("trust can be turned off")
	}
}

func TestForwardingHeadersCannotBorrowTheOwnersDevice(t *testing.T) {
	m := newMachine(t, "hub")
	m.s.handler.tail.source = func() (tailnetView, error) {
		return tailnetView{User: "u-me", Login: "me@example.com", Peers: []tailnetPeer{{Host: "pc", IPs: []string{"100.1.1.1"}, User: "u-me", Online: true}}}, nil
	}
	session := func(remote string, headers map[string]string) bool {
		r := httptest.NewRequest("GET", "/api/session", nil)
		r.RemoteAddr = remote
		for k, v := range headers {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		m.s.Router().ServeHTTP(w, r)
		var out map[string]bool
		json.Unmarshal(w.Body.Bytes(), &out)
		return out["connected"]
	}
	if session("192.168.0.9:4000", map[string]string{"X-Forwarded-For": "100.1.1.1", "X-Real-IP": "100.1.1.1"}) {
		t.Fatal("a LAN device claiming the owner's tailnet address must not sign in")
	}
	if session("192.168.0.9:4000", map[string]string{"Tailscale-User-Login": "me@example.com"}) {
		t.Fatal("the identity header counts only from tailscale serve on this machine")
	}
	if !session("100.1.1.1:4000", nil) {
		t.Fatal("the owner's own device connecting directly should sign in")
	}
}
