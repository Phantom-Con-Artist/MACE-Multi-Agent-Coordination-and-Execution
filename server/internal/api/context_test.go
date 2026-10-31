package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fetchContext(tm *team, agent, token, room, view, q string) (int, string) {
	path := "/api/v1/ctx/" + room + "/" + view
	if q != "" {
		path += "?q=" + q
	}
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Mace-Agent", agent)
	w := httptest.NewRecorder()
	tm.s.Router().ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func TestContextFetchesAreScopedAndNeverRepeated(t *testing.T) {
	tm := newTeam(t, "ari", "bo", "cy")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "[plan] ari takes the parser, @agy-2 takes the UI")
	tm.set("bo", "reply.txt", "[PASS]")
	tm.say(t, room.ID, "@ari split the calculator work")
	settled(t, tm.s, room.ID)

	ctxs := tm.s.handler.ctx
	bo, cy := tm.agents["bo"].ID, tm.agents["cy"].ID
	code, body := fetchContext(tm, bo, ctxs.token(bo), room.ID, "plan", "")
	if code != 200 || !strings.Contains(body, "[plan] ari takes the parser") || !strings.Contains(body, "split the calculator work") {
		t.Fatalf("plan should hold the owner's request and the plan: %d %s", code, body)
	}
	if _, again := fetchContext(tm, bo, ctxs.token(bo), room.ID, "plan", ""); !strings.Contains(again, "Unchanged") {
		t.Fatalf("an unchanged repeat fetch should not resend the text: %s", again)
	}
	if code, body := fetchContext(tm, bo, ctxs.token(bo), room.ID, "search", "parser"); code != 200 || !strings.Contains(body, "parser") {
		t.Fatalf("search should find the plan: %d %s", code, body)
	}
	if code, _ := fetchContext(tm, cy, ctxs.token(cy), room.ID, "plan", ""); code != 403 {
		t.Fatalf("a non-member must not read the group, got %d", code)
	}
	if code, _ := fetchContext(tm, bo, ctxs.token(cy), room.ID, "plan", ""); code != 401 {
		t.Fatalf("one agent's token must not work for another, got %d", code)
	}
}

func TestResumedTurnsSkipTheUnchangedHeader(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	room := tm.group(t, "ari", "bo")
	tm.set("ari", "reply.txt", "on it")
	tm.set("bo", "reply.txt", "[PASS]")
	turns := func() []string { return strings.Split(tm.promptsOf(t, "ari"), "----\n") }

	tm.say(t, room.ID, "@ari first task")
	settled(t, tm.s, room.ID)
	tm.say(t, room.ID, "@ari second task")
	settled(t, tm.s, room.ID)
	second := turns()[1]
	if strings.Contains(second, "HOW THIS TEAM WORKS") || strings.Contains(second, "Your conversations:") {
		t.Fatalf("a resumed turn should not repeat the unchanged header or awareness:\n%s", second)
	}
	if !strings.Contains(second, "second task") || !strings.Contains(second, "unchanged since your last turn") {
		t.Fatalf("a resumed turn still carries the request and says the rest is unchanged:\n%s", second)
	}
	// A new role sends only that coworker's changed line.
	call(t, tm.s, "POST", "/api/v1/rooms/"+room.ID+"/members/"+tm.agents["bo"].ID+"/role", map[string]string{"role": "Tester"}, nil)
	tm.say(t, room.ID, "@ari third task")
	settled(t, tm.s, room.ID)
	third := turns()[2]
	if strings.Contains(third, "HOW THIS TEAM WORKS") || !strings.Contains(third, "TEAM CHANGES") || !strings.Contains(third, "Tester") {
		t.Fatalf("a changed role should arrive as one team line:\n%s", third)
	}
	if strings.Count(third, "\n- ") != 1 {
		t.Fatalf("only the changed coworker's line should be sent:\n%s", third)
	}
	// The next turn is back to the short form.
	tm.say(t, room.ID, "@ari fourth task")
	settled(t, tm.s, room.ID)
	if fourth := turns()[3]; strings.Contains(fourth, "TEAM CHANGES") || !strings.Contains(fourth, "unchanged since your last turn") {
		t.Fatalf("the change was delivered once:\n%s", fourth)
	}
}

func TestAgentsGetTheirOwnContextAccess(t *testing.T) {
	tm := newTeam(t, "ari")
	tm.s.handler.ctx.url = "http://127.0.0.1:9"
	bin := filepath.Dir(tm.timeline)
	os.WriteFile(filepath.Join(bin, "mace"), []byte("#!/bin/sh\n"), 0700)
	room := tm.group(t, "ari")
	tm.say(t, room.ID, "@ari hello")
	settled(t, tm.s, room.ID)
	env, _ := os.ReadFile(filepath.Join(tm.dirs["ari"], "env.log"))
	id := tm.agents["ari"].ID
	for _, want := range []string{"MACE_CTX_AGENT=" + id, "MACE_CTX_TOKEN=" + tm.s.handler.ctx.token(id), "MACE_CTX_CHAT=" + room.ID} {
		if !strings.Contains(string(env), want) {
			t.Errorf("the agent's process is missing %s:\n%s", want, env)
		}
	}
	if strings.Contains(string(env), "MACE_PROCESS_TAG=") && strings.Contains(string(env), "MACE_AUTH") {
		t.Error("only context settings may reach the agent")
	}
	if !strings.Contains(tm.promptsOf(t, "ari"), "mace ctx team") {
		t.Error("the turn should say how to fetch more context")
	}
}

func TestCodeGraphsAreBuiltWithoutAModelAndAdvertised(t *testing.T) {
	tm := newTeam(t, "ari")
	dir := tm.dirs["ari"]
	os.MkdirAll(filepath.Join(dir, ".git", "info"), 0755)
	fake := filepath.Join(filepath.Dir(tm.timeline), "graphify")
	os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@ backend=$GRAPHIFY_BACKEND\" >> \"$2/graphify.log\"\nmkdir -p \"$2/graphify-out\" && echo '{}' > \"$2/graphify-out/graph.json\"\n"), 0700)
	tm.s.handler.graphs.command = fake
	room := tm.group(t, "ari")

	tm.say(t, room.ID, "@ari look around")
	settled(t, tm.s, room.ID)
	waitFor(t, "graph built", func() bool { _, err := os.Stat(graphFile(dir)); return err == nil })
	log, _ := os.ReadFile(filepath.Join(dir, "graphify.log"))
	if !strings.Contains(string(log), "update "+dir+" backend=none") {
		t.Fatalf("graphify should run its code pass with no model backend: %s", log)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude")); !strings.Contains(string(b), "graphify-out/") {
		t.Fatal("the graph output should be excluded from git status")
	}
	tm.say(t, room.ID, "@ari now fix it")
	settled(t, tm.s, room.ID)
	if !strings.Contains(tm.promptsOf(t, "ari"), "CODE MAP") {
		t.Fatal("once the graph exists, turns should point the agent at it")
	}
	home, _ := os.UserHomeDir()
	if graphable("/") || graphable(home) {
		t.Fatal("the filesystem root and home folder are never indexed")
	}
}
