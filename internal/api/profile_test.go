package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileRenameRoleAndColor(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	ari, bo := tm.agents["ari"], tm.agents["bo"]
	var out Session
	if call(t, tm.s, "POST", "/api/v1/sessions/"+ari.ID+"/profile", map[string]any{"name": "Ari the Reviewer", "handle": "reviewer", "role": "Review every change for security", "color": 3}, &out) != 200 {
		t.Fatal("profile update failed")
	}
	if out.Name != "Ari the Reviewer" || out.Handle != "reviewer" || out.Role == "" || out.Color == nil || *out.Color != 3 {
		t.Fatalf("profile not applied: %+v", out)
	}
	if code := call(t, tm.s, "POST", "/api/v1/sessions/"+bo.ID+"/profile", map[string]any{"handle": "Reviewer"}, nil); code != 409 {
		t.Fatalf("a taken handle must be refused, got %d", code)
	}
	if code := call(t, tm.s, "POST", "/api/v1/sessions/"+bo.ID+"/profile", map[string]any{"name": "  "}, nil); code != 400 {
		t.Fatalf("an empty name must be refused, got %d", code)
	}

	// The role set in private applies in a group, until the group sets its own.
	room := tm.group(t, "ari", "bo")
	var g Room
	tm.s.handler.store.Document(context.Background(), "room", room.ID, &g)
	header := tm.s.handler.conversationHeader(context.Background(), room.ID, bo.ID, g.Members, &g, Policy{Mode: "reply", Limit: 2})
	if !strings.Contains(header, "@reviewer") || !strings.Contains(header, "Review every change for security") {
		t.Fatalf("the profile name and role should reach the group:\n%s", header)
	}
	call(t, tm.s, "POST", "/api/v1/rooms/"+room.ID+"/members/"+ari.ID+"/role", map[string]string{"role": "Write the release notes"}, nil)
	tm.s.handler.store.Document(context.Background(), "room", room.ID, &g)
	header = tm.s.handler.conversationHeader(context.Background(), room.ID, bo.ID, g.Members, &g, Policy{Mode: "reply", Limit: 2})
	if !strings.Contains(header, "Write the release notes") || strings.Contains(header, "Review every change") {
		t.Fatal("a group role overrides the profile role in that group")
	}
	tm.say(t, ari.ID, "hi")
	settled(t, tm.s, ari.ID)
	if p := tm.prompts("ari"); !strings.Contains(p[len(p)-1], "Your role, set by the owner: Review every change for security") {
		t.Fatal("the agent should be told its own role")
	}
}

func TestOwnerProvidedAvatars(t *testing.T) {
	home := t.TempDir()
	t.Setenv("MACE_HOME", home)
	os.MkdirAll(filepath.Join(home, "avatars"), 0700)
	os.WriteFile(filepath.Join(home, "avatars", "agy.svg"), []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600)
	tm := newTeam(t, "ari")
	r := httptest.NewRequest("GET", "/api/v1/avatars/agy", nil)
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	tm.s.Router().ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("logo not served safely: %d %v", w.Code, w.Header())
	}
	for _, bad := range []string{"codex", "..%2Fsecret", "AGY"} {
		r = httptest.NewRequest("GET", "/api/v1/avatars/"+bad, nil)
		r.Header.Set("Authorization", "Bearer secret")
		w = httptest.NewRecorder()
		tm.s.Router().ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("%s should not be served, got %d", bad, w.Code)
		}
	}
	var list []Session
	call(t, tm.s, "GET", "/api/v1/sessions", nil, &list)
	if len(list) != 1 || !list[0].Avatar {
		t.Fatal("sessions should report the owner-provided logo")
	}
}
