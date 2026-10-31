package api

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func parseFile(t *testing.T, path string, parse func(string, *turnState)) *turnState {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	state := &turnState{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		parse(scanner.Text(), state)
	}
	return state
}

// The fixtures are trimmed recordings of real agy and muse runs (2026-10-31)
// that read a file and answer with its second line.
func TestAdaptersParseRecordedStreams(t *testing.T) {
	for _, c := range []struct {
		file, session string
		parse         func(string, *turnState)
	}{
		{"testdata/agy-stream.jsonl", "8fd87ec8-2b36-4163-b02f-fec3c77be9cc", parseAgy},
		{"testdata/muse-stream.jsonl", "01a19096-6d10-7d23-86ad-cfc24a1d8ff8", parseMuse},
	} {
		state := parseFile(t, c.file, c.parse)
		if state.SessionID != c.session || state.reply() != "beta" || state.Err != "" {
			t.Fatalf("%s: session=%q reply=%q err=%q", c.file, state.SessionID, state.reply(), state.Err)
		}
		if len(state.Steps) != 1 || state.Steps[0].Kind != "read" || state.Steps[0].State != "done" || !strings.Contains(state.Steps[0].Title, "notes.txt") {
			t.Fatalf("%s: expected one finished read of notes.txt, got %+v", c.file, state.Steps)
		}
	}
}

func TestStepTitlesShowFileNames(t *testing.T) {
	for in, want := range map[string]string{
		"Read text file `/tmp/x/project/PLAN.md`": "Read `PLAN.md`",
		"Ran `go test ./...`":                      "Ran `go test ./...`",
		"Edited `main.go`":                         "Edited `main.go`",
	} {
		if got := shortTitle(in); got != want {
			t.Fatalf("shortTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAdaptersParseDocumentedFormats(t *testing.T) {
	claude := []string{
		`{"type":"system","subtype":"init","session_id":"c-1"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Edit","input":{"file_path":"/repo/main.go"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t2","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t2","is_error":true}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Done.","session_id":"c-1"}`,
	}
	state := &turnState{}
	for _, l := range claude {
		parseClaude(l, state)
	}
	if state.SessionID != "c-1" || state.reply() != "Done." || len(state.Steps) != 2 ||
		state.Steps[0].Title != "Edited `main.go`" || state.Steps[0].State != "done" ||
		state.Steps[1].Title != "Ran `go test ./...`" || state.Steps[1].State != "failed" {
		t.Fatalf("claude parse: %+v %q", state.Steps, state.reply())
	}
	codex := []string{
		`{"type":"thread.started","thread_id":"x-9"}`,
		`{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"ls -la"}}`,
		`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"ls -la","status":"completed"}}`,
		`{"type":"item.completed","item":{"id":"i2","type":"file_change","changes":[{"path":"/r/a.go","kind":"update"}]}}`,
		`{"type":"item.completed","item":{"id":"i3","type":"agent_message","text":"All set."}}`,
	}
	state = &turnState{}
	for _, l := range codex {
		parseCodex(l, state)
	}
	if state.SessionID != "x-9" || state.reply() != "All set." || len(state.Steps) != 2 || state.Steps[0].State != "done" || state.Steps[1].Title != "Edited `a.go`" {
		t.Fatalf("codex parse: %+v %q", state.Steps, state.reply())
	}
}

// fakeCLI installs a script that records its arguments and prints a fixture.
func fakeCLI(t *testing.T, dir, name, fixture string) string {
	t.Helper()
	args := filepath.Join(dir, name+".args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"" + args + "\"\necho ---- >> \"" + args + "\"\ncat \"" + fixture + "\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return args
}

func recordedCalls(path string) []string {
	b, _ := os.ReadFile(path)
	return strings.Split(strings.TrimSuffix(string(b), "----\n"), "----\n")
}

func TestTurnsResumeTheSameNativeSession(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	wd, _ := os.Getwd()
	agyArgs := fakeCLI(t, dir, "agy", filepath.Join(wd, "testdata/agy-stream.jsonl"))
	// Real muse reports the session ID it was given; the fake does the same.
	museArgs := filepath.Join(dir, "muse.args")
	museScript := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"" + museArgs + "\"\necho ---- >> \"" + museArgs + "\"\n" +
		"sed \"s/01a19096-6d10-7d23-86ad-cfc24a1d8ff8/$4/g\" \"" + filepath.Join(wd, "testdata/muse-stream.jsonl") + "\"\n"
	os.WriteFile(filepath.Join(dir, "muse"), []byte(museScript), 0700)
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	for _, provider := range []string{"agy", "muse"} {
		var a Session
		if call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": provider}, &a) != 201 || a.Handle != provider {
			t.Fatalf("%s: session or handle wrong: %+v", provider, a)
		}
		var streamed [][]Activity
		live := func(steps []Activity) { streamed = append(streamed, steps) }
		first, err := s.handler.turn(context.Background(), a.ID, turnPrompt{Full: "FULL-CONTEXT", Delta: "ONLY-NEW"}, "local", live)
		if err != nil || first.Text != "beta" || len(first.Activity) != 1 || first.Reset {
			t.Fatalf("%s first turn: %+v %v", provider, first, err)
		}
		if len(streamed) == 0 {
			t.Fatalf("%s: no live activity was published", provider)
		}
		second, err := s.handler.turn(context.Background(), a.ID, turnPrompt{Full: "FULL-CONTEXT", Delta: "ONLY-NEW"}, "local", nil)
		if err != nil || second.Text != "beta" {
			t.Fatalf("%s second turn: %+v %v", provider, second, err)
		}
		store.Document(context.Background(), "session", a.ID, &a)
		if a.NativeSession == "" || a.Status != "ready" {
			t.Fatalf("%s: native session not recorded: %+v", provider, a)
		}
	}
	agy := recordedCalls(agyArgs)
	if len(agy) != 2 || strings.Contains(agy[0], "--conversation") || !strings.Contains(agy[1], "--conversation\n8fd87ec8-2b36-4163-b02f-fec3c77be9cc") {
		t.Fatalf("agy should start fresh, then resume its conversation:\n%v", agy)
	}
	if !strings.Contains(agy[0], "FULL-CONTEXT") || !strings.Contains(agy[0], "Multi Agent Coordination") {
		t.Fatal("a fresh session needs the instructions and the full context")
	}
	if !strings.Contains(agy[1], "ONLY-NEW") || strings.Contains(agy[1], "FULL-CONTEXT") || strings.Contains(agy[1], "Multi Agent Coordination") {
		t.Fatal("a resumed session should only receive what is new")
	}
	if !strings.Contains(agy[1], "MACE AWARENESS") {
		t.Fatal("every turn should carry current awareness")
	}
	muse := recordedCalls(museArgs)
	id := func(call string) string {
		lines := strings.Split(call, "\n")
		for i, l := range lines {
			if l == "--session-id" && i+1 < len(lines) {
				return lines[i+1]
			}
		}
		return ""
	}
	if len(muse) != 2 || id(muse[0]) == "" || id(muse[0]) != id(muse[1]) {
		t.Fatalf("muse should reuse one chosen session ID: %q", muse)
	}
}

// agy starts a new conversation when the requested one is gone. Mace must
// notice and report the reset instead of pretending the agent remembers.
func TestTurnReportsSilentSessionReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	wd, _ := os.Getwd()
	fakeCLI(t, dir, "agy", filepath.Join(wd, "testdata/agy-stream.jsonl"))
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	var a Session
	call(t, s, "POST", "/api/v1/sessions", map[string]string{"provider": "agy"}, &a)
	s.handler.updateSession(context.Background(), a.ID, func(s *Session) { s.NativeSession, s.NativeDir = "gone-conversation", s.Directory })
	res, err := s.handler.turn(context.Background(), a.ID, turnPrompt{Full: "x"}, "local", nil)
	if err != nil || !res.Reset {
		t.Fatalf("expected a reported reset, got %+v %v", res, err)
	}
	store.Document(context.Background(), "session", a.ID, &a)
	if a.NativeSession != "8fd87ec8-2b36-4163-b02f-fec3c77be9cc" {
		t.Fatalf("the new conversation should be recorded, got %q", a.NativeSession)
	}
}
