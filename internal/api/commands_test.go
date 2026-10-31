package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func command(t *testing.T, tm *team, id, name, arg string) (int, commandResult) {
	t.Helper()
	var out commandResult
	code := call(t, tm.s, "POST", "/api/v1/sessions/"+id+"/command", map[string]string{"name": name, "arg": arg}, &out)
	return code, out
}

func TestCommandsChangeModelFolderAndHandle(t *testing.T) {
	tm := newTeam(t, "ari", "bo")
	ari := tm.agents["ari"]

	code, out := command(t, tm, ari.ID, "model", "gemini-3.8-flash-low")
	if code != 200 || out.Session.Model != "gemini-3.8-flash-low" || !strings.Contains(out.Note.Text, "gemini-3.8-flash-low") {
		t.Fatalf("model not set: %d %+v", code, out)
	}
	// The model is the agent's own, so every conversation it joins sees it.
	if a := tm.s.handler.awareness(context.Background(), out.Session); !strings.Contains(a, "Model: gemini-3.8-flash-low") {
		t.Fatalf("awareness should carry the model:\n%s", a)
	}
	if code, _ := command(t, tm, ari.ID, "model", "bad model; rm -rf"); code != 400 {
		t.Fatalf("a malformed model must be refused, got %d", code)
	}
	if code, out := command(t, tm, ari.ID, "model", "default"); code != 200 || out.Session.Model != "" {
		t.Fatalf("default should clear the model: %d %+v", code, out.Session)
	}

	dir := filepath.Join(t.TempDir(), "work")
	os.Mkdir(dir, 0700)
	code, out = command(t, tm, ari.ID, "cd", dir)
	if code != 200 || out.Session.Directory != dir {
		t.Fatalf("folder not changed: %d %+v", code, out.Session)
	}
	if code, _ := command(t, tm, ari.ID, "cd", filepath.Join(dir, "missing")); code != 400 {
		t.Fatalf("a missing folder must be refused, got %d", code)
	}

	if code, out := command(t, tm, ari.ID, "rename", "Reviewer"); code != 200 || out.Session.Handle != "reviewer" {
		t.Fatalf("handle not changed: %d %+v", code, out.Session)
	}
	if code, _ := command(t, tm, tm.agents["bo"].ID, "rename", "reviewer"); code != 409 {
		t.Fatalf("a taken handle must be refused, got %d", code)
	}
	if code, _ := command(t, tm, ari.ID, "dance", ""); code != 400 {
		t.Fatalf("unknown commands must be refused, got %d", code)
	}
}

func TestCommandNotesStayOutOfTheAgentsTranscript(t *testing.T) {
	tm := newTeam(t, "ari")
	ari := tm.agents["ari"]
	tm.s.handler.updateSession(context.Background(), ari.ID, func(s *Session) { s.NativeSession, s.NativeDir = "native-1", s.Directory })

	code, out := command(t, tm, ari.ID, "new", "")
	if code != 200 || out.Session.NativeSession != "" {
		t.Fatalf("/new should drop the native session: %d %+v", code, out.Session)
	}
	if _, out := command(t, tm, ari.ID, "status", ""); !strings.Contains(out.Note.Text, "Folder") || out.Note.Kind != "note" {
		t.Fatalf("status note: %+v", out.Note)
	}

	history := tm.history(t, ari.ID)
	if len(history) != 2 || history[0].Kind != "note" || history[0].Sender != "mace" {
		t.Fatalf("command results should be notes in the private chat: %+v", history)
	}
	rows, _, _ := tm.s.handler.store.Messages(context.Background(), ari.ID, 0, 0, 40)
	raw := []json.RawMessage{}
	for _, r := range rows {
		raw = append(raw, r.Data)
	}
	if got := tm.s.handler.transcript(context.Background(), raw, ari.ID); got != "" {
		t.Fatalf("notes must not reach the agent, got:\n%s", got)
	}
}

func TestModelsListComesFromTheCLIWhereOffered(t *testing.T) {
	tm := newTeam(t, "ari")
	listModels = func(ctx context.Context, provider string) ([]modelChoice, string, error) {
		return []modelChoice{{ID: "gemini-3.8-flash-low", Label: "Gemini 3.8 Flash (Low)"}}, "cli", nil
	}
	t.Cleanup(func() { listModels = providerModels })
	var out modelList
	if call(t, tm.s, "GET", "/api/v1/sessions/"+tm.agents["ari"].ID+"/models", nil, &out) != 200 || len(out.Models) != 1 || out.Source != "cli" {
		t.Fatalf("models: %+v", out)
	}
}

func TestParseModelLinesSkipsTheBanner(t *testing.T) {
	// Captured from agy models on 2026-10-31.
	out := "Fetching available models...\ngemini-3.8-flash-high\tGemini 3.8 Flash (High)\nclaude-opus-4-6-thinking\tClaude Opus 4.6 (Thinking)\n"
	got := parseModelLines(out)
	if len(got) != 2 || got[0].ID != "gemini-3.8-flash-high" || got[1].Label != "Claude Opus 4.6 (Thinking)" {
		t.Fatalf("parsed %+v", got)
	}
}
