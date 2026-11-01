package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// withSessions turns sessions on for a test machine and counts restarts.
func (m *machine) withSessions(t *testing.T) *atomic.Int32 {
	t.Helper()
	h := m.s.handler
	h.dataDir = t.TempDir()
	ctx := context.Background()
	if _, err := h.store.AddWorkspace(ctx, "default", "Default"); err != nil {
		t.Fatal(err)
	}
	h.store.SetActiveWorkspace(ctx, "default", time.Now().Add(-time.Hour))
	restarts := &atomic.Int32{}
	h.restart = func() { restarts.Add(1) }
	return restarts
}

func TestANewSessionIsFollowedByLinkedMachines(t *testing.T) {
	a, b := newMachine(t, "laptop"), newMachine(t, "pc")
	joinAll(t, a, b)
	ra, rb := a.withSessions(t), b.withSessions(t)

	var ws struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if c := a.owner(t, "POST", "/api/v1/workspaces", map[string]string{"name": "Launch"}, &ws); c != 200 || ws.Name != "Launch" {
		t.Fatalf("new session: %d %+v", c, ws)
	}
	ctx := context.Background()
	deadline := time.Now().Add(3 * time.Second)
	for b.s.handler.store.ActiveSince(ctx).ID != ws.ID && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if a.s.handler.store.ActiveSince(ctx).ID != ws.ID || b.s.handler.store.ActiveSince(ctx).ID != ws.ID {
		t.Fatal("both machines should run the new session")
	}
	time.Sleep(800 * time.Millisecond)
	if ra.Load() != 1 || rb.Load() != 1 {
		t.Fatalf("each machine should restart once into it: %d %d", ra.Load(), rb.Load())
	}
	// An older choice from another machine does not pull a machine back.
	b.s.handler.followWorkspace(ctx, workspaceSwitch{ID: "default", Name: "Default", At: time.Now().Add(-time.Minute)})
	if b.s.handler.store.ActiveSince(ctx).ID != ws.ID {
		t.Fatal("a stale switch must not win")
	}
	// IDs from other machines become file names; anything odd is refused.
	b.s.handler.followWorkspace(ctx, workspaceSwitch{ID: "../../etc", Name: "x", At: time.Now()})
	if b.s.handler.store.ActiveSince(ctx).ID != ws.ID {
		t.Fatal("a malformed session ID must be refused")
	}
	// A status from a machine still in the old session must not appear.
	a.s.handler.statusReceive(ctx, Status{ID: "s0ld000", Home: b.s.handler.selfID(), Session: "default", Text: "old", Expires: time.Now().Add(time.Hour), Updated: time.Now()})
	if _, ok := a.s.handler.statusGet(ctx, "s0ld000"); ok {
		t.Fatal("a status from another session must not leak into this one")
	}
	var list []workspaceView
	a.owner(t, "GET", "/api/v1/workspaces", nil, &list)
	if len(list) != 2 {
		t.Fatalf("the old session stays saved: %+v", list)
	}
	if c := a.owner(t, "POST", "/api/v1/workspaces/"+ws.ID+"/remove", map[string]any{}, nil); c != 409 {
		t.Fatalf("the active session cannot be removed: %d", c)
	}
	if c := a.owner(t, "POST", "/api/v1/workspaces/default/remove", map[string]any{}, nil); c != 200 {
		t.Fatalf("an old session can be removed: %d", c)
	}
}
