package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func signInWith(m *machine, code string) int {
	b, _ := json.Marshal(map[string]string{"code": code})
	r := httptest.NewRequest("POST", "/api/session", bytes.NewReader(b))
	w := httptest.NewRecorder()
	m.s.Router().ServeHTTP(w, r)
	return w.Code
}

func TestSignInCodesWorkOnceWithoutTheToken(t *testing.T) {
	m := newMachine(t, "hub")
	var qr struct {
		Link string `json:"link"`
		SVG  string `json:"svg"`
	}
	if c := m.owner(t, "GET", "/api/v1/signin/qr", nil, &qr); c != 200 || !strings.HasPrefix(qr.SVG, "<svg") {
		t.Fatalf("qr: %d", c)
	}
	if !strings.HasPrefix(qr.Link, m.ts.URL+"/#connect-once=") || strings.Contains(qr.Link, "secret-hub") {
		t.Fatalf("the link should hold a one-time code, never the token: %s", qr.Link)
	}
	code := qr.Link[strings.Index(qr.Link, "=")+1:]
	if c := signInWith(m, code); c != 200 {
		t.Fatalf("a fresh code should sign in: %d", c)
	}
	if c := signInWith(m, code); c != 401 {
		t.Fatalf("a used code must be refused: %d", c)
	}
}

func TestThePhoneSwitchesMachinesAlreadySignedIn(t *testing.T) {
	hub, worker := newMachine(t, "hub"), newMachine(t, "worker")
	joinAll(t, hub, worker)
	var out struct {
		URL string `json:"url"`
	}
	if c := hub.owner(t, "POST", "/api/v1/nodes/"+worker.s.handler.selfNode().ID+"/open", map[string]any{}, &out); c != 200 {
		t.Fatalf("open: %d", c)
	}
	if !strings.HasPrefix(out.URL, worker.ts.URL+"/#connect-once=") {
		t.Fatalf("the link should point at the worker: %s", out.URL)
	}
	if c := signInWith(worker, out.URL[strings.Index(out.URL, "=")+1:]); c != 200 {
		t.Fatalf("the worker should accept its own one-time code: %d", c)
	}
	// An owner request without a node signature cannot mint codes on a node.
	if c := worker.owner(t, "POST", "/api/v1/node/signin", map[string]any{}, nil); c != 401 {
		t.Fatalf("node sign-in links are only for joined machines: %d", c)
	}
}

func TestOnlyMessagesThatNeedTheOwnerNotify(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	h := tm.s.handler
	var mu sync.Mutex
	sent := []string{}
	h.push.send = func(_ context.Context, payload []byte, _ pushSubscription, _ pushKeys) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		sent = append(sent, string(payload))
		return 201, nil
	}
	sub := map[string]any{"endpoint": "https://push.example/abc", "keys": map[string]string{"auth": "a", "p256dh": "p"}}
	if c := call(t, tm.s, "POST", "/api/v1/push/subscribe", sub, nil); c != 201 {
		t.Fatalf("subscribe: %d", c)
	}
	ari := tm.agents["ari"].ID
	post := func(m Message) {
		m.ID, m.At = newID(), time.Now()
		h.postMessage(context.Background(), &m)
	}
	post(Message{Room: room.ID, Sender: ari, Text: "lol same"})
	post(Message{Room: room.ID, Sender: ari, Text: "which db should we use?", Flair: "question"})
	post(Message{Room: ari, Sender: ari, Text: "private reply"})
	post(Message{Room: room.ID, Sender: "you", Text: "my own message"})
	waitFor(t, "two notifications", func() bool { mu.Lock(); defer mu.Unlock(); return len(sent) >= 2 })
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(sent) != 2 || !strings.Contains(strings.Join(sent, ""), "which db") || !strings.Contains(strings.Join(sent, ""), "private reply") {
		t.Fatalf("only the question and the private reply should notify: %v", sent)
	}
}
