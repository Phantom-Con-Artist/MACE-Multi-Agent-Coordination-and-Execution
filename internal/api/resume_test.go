package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestARemoteTurnSurvivesADroppedConnectionAndRunsOnce(t *testing.T) {
	hub, worker, agent, dir := hubAndWorker(t)
	os.WriteFile(filepath.Join(dir, "delay"), []byte("1"), 0600)
	node, _, _ := strings.Cut(agent, "::")
	hub.owner(t, "GET", "/api/v1/peers/"+node+"/sessions", nil, &[]Session{})

	// The first turn request reaches the worker, then the connection drops,
	// as when Wi-Fi blinks or the machine sleeps for a moment.
	inner := worker.ts.Config.Handler
	var dropped atomic.Bool
	worker.ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/turn") || !dropped.CompareAndSwap(false, true) {
			inner.ServeHTTP(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		ctx, cancel := context.WithCancel(context.Background())
		req := r.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		go inner.ServeHTTP(httptest.NewRecorder(), req)
		time.Sleep(200 * time.Millisecond)
		cancel()
		conn, _, _ := w.(http.Hijacker).Hijack()
		conn.Close()
	})

	if c := hub.owner(t, "POST", "/api/v1/chats/"+agent+"/messages", map[string]string{"text": "build it"}, nil); c != 201 {
		t.Fatalf("send: %d", c)
	}
	settled(t, hub.s, agent)
	var msgs []Message
	hub.owner(t, "GET", "/api/v1/chats/"+agent+"/messages", nil, &msgs)
	if len(msgs) != 2 || !strings.Contains(msgs[1].Text, "hello from the worker") {
		t.Fatalf("the hub should get the reply after the drop: %+v", msgs)
	}
	timeline, _ := os.ReadFile(os.Getenv("TIMELINE"))
	if n := strings.Count(string(timeline), "start "); n != 1 || !dropped.Load() {
		t.Fatalf("the agent should run once across the retry, ran %d times:\n%s", n, timeline)
	}
}

func TestKeyedTurnsOutliveTheirCallerUntilStopped(t *testing.T) {
	var rr remoteRuns
	var started atomic.Int32
	run := func(ctx context.Context) (turnResult, error) {
		started.Add(1)
		<-ctx.Done()
		return turnResult{}, ctx.Err()
	}
	caller, leave := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); leave() }()
	if _, err := rr.join(caller, "k", run); !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller leaving should end its wait: %v", err)
	}
	// The turn keeps running; a second ask with the same key joins it.
	done := make(chan error, 1)
	go func() { _, err := rr.join(context.Background(), "k", run); done <- err }()
	time.Sleep(50 * time.Millisecond)
	rr.stop("k")
	if err := <-done; !errors.Is(err, context.Canceled) || started.Load() != 1 {
		t.Fatalf("stop should end the one running turn: %v, started %d", err, started.Load())
	}
}

func TestBackoffStaysWithinItsCap(t *testing.T) {
	for attempt := range 12 {
		for range 50 {
			if d := backoff(attempt); d <= 0 || d > 31*time.Second {
				t.Fatalf("attempt %d waited %v", attempt, d)
			}
		}
	}
}

func TestNearCopiesCountAsRepeatsButNewNewsDoesNot(t *testing.T) {
	same := [][2]string{
		{"@lead ready to preview, build is green", "@lead ready to preview — build is green!"},
		{"Waiting on @agy-pc1 for the DynamicPulp v2 pass before I wire it in", "waiting on @agy-pc1 for the DynamicPulp v2 pass before i wire it in."},
	}
	different := [][2]string{
		// Real consecutive messages from one agent in a team run.
		{"Finished DynamicPulp.tsx with expanding pill, live equalizer, boil timer, PulpDrop transfer, and CitrusFace call modes.  @lead @muse",
			"@agy @muse DynamicPulp is already shared and waiting in your inbox! Exported both named `{ DynamicPulp }` and default, self-contained with Tailwind. Ready for `App.tsx`."},
		{"@lead data file cooked, keynote copy + A18 Zest specs + pricing + citrus lore in there", "@lead copy v2 done, killed eyebrows + hype caps, tight 2-line Apple cadence"},
		{"build green", "build red: App.tsx fails to import PhoneCustomizer"},
	}
	for _, p := range same {
		if s := similarity(shingles(p[0]), shingles(p[1])); s < repeatSimilarity {
			t.Errorf("%.2f should count as a repeat: %q", s, p[1])
		}
	}
	for _, p := range different {
		if s := similarity(shingles(p[0]), shingles(p[1])); s >= repeatSimilarity {
			t.Errorf("%.2f is new news, not a repeat: %q", s, p[1])
		}
	}
}

func TestAgentsOnAWorkerGetTheShortBriefingToo(t *testing.T) {
	hub, _, agent, dir := hubAndWorker(t)
	node, _, _ := strings.Cut(agent, "::")
	hub.owner(t, "GET", "/api/v1/peers/"+node+"/sessions", nil, &[]Session{})
	var room Room
	if c := hub.owner(t, "POST", "/api/v1/rooms", map[string]any{"name": "Team", "members": []string{agent}}, &room); c != 201 {
		t.Fatalf("create group: %d", c)
	}
	for _, text := range []string{"first task", "second task"} {
		hub.owner(t, "POST", "/api/v1/chats/"+room.ID+"/messages", map[string]string{"text": text}, nil)
		settled(t, hub.s, room.ID)
	}
	log, _ := os.ReadFile(filepath.Join(dir, "prompts.log"))
	turns := strings.Split(string(log), "----\n")
	if len(turns) < 2 {
		t.Fatalf("expected two turns on the worker:\n%s", log)
	}
	second := turns[1]
	if strings.Contains(second, "HOW THIS TEAM WORKS") || !strings.Contains(second, "unchanged since your last turn") || !strings.Contains(second, "second task") {
		t.Fatalf("the worker's agent already holds the rules:\n%s", second)
	}
}

func TestTheHubRecordsWhatAgentsOnOtherMachinesSpend(t *testing.T) {
	hub, _, agent, dir := hubAndWorker(t)
	node, _, _ := strings.Cut(agent, "::")
	hub.owner(t, "GET", "/api/v1/peers/"+node+"/sessions", nil, &[]Session{})
	// Two model calls, the second reported twice as Antigravity does.
	call := `{"event":"step_update","step_update":{"step_index":%d,"state":"DONE","step_type":"agent_response","usage":{"input_tokens":%d,"output_tokens":%d,"thinking_tokens":5,"cache_read_tokens":%d}}}`
	os.WriteFile(filepath.Join(dir, "usage"), []byte(fmt.Sprintf(call, 1, 1000, 100, 400)+"\n"+fmt.Sprintf(call, 3, 1200, 50, 0)+"\n"+fmt.Sprintf(call, 3, 1200, 50, 0)+"\n"), 0600)

	hub.owner(t, "POST", "/api/v1/chats/"+agent+"/messages", map[string]string{"text": "go"}, nil)
	run := settled(t, hub.s, agent)
	if run.Usage.Input != 2200 || run.Usage.Output != 150 || run.Usage.Cached != 400 || run.Usage.Calls != 2 {
		t.Fatalf("the run should count the worker's two calls once each: %+v", run.Usage)
	}
	var rows []struct {
		Name  string `json:"name"`
		Today Usage  `json:"today"`
		Total Usage  `json:"total"`
	}
	hub.owner(t, "GET", "/api/v1/usage", nil, &rows)
	if len(rows) != 1 || rows[0].Today.Input != 2200 || rows[0].Total.Turns != 1 || !strings.Contains(rows[0].Name, "worker") {
		t.Fatalf("the hub should list the worker agent's spending: %+v", rows)
	}
	var found int
	hub.s.handler.store.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE action='TURN_USAGE' AND details_json LIKE '%"machine":"worker"%'`).Scan(&found)
	if found != 1 {
		t.Fatal("the audit log should name the machine that spent it")
	}
}

func TestEachCLIsUsageIsRead(t *testing.T) {
	var muse turnState
	for _, total := range []int{10, 40} { // Muse repeats its running total
		parseMuse(fmt.Sprintf(`{"payload_type":"run.usage","payload":{"allowance":{"weekly":{"used_percent":29}},"models":[{"input_tokens":%d,"cached_tokens":5,"output_tokens":3,"reasoning_tokens":1,"calls":2}]}}`, total), &muse)
	}
	if muse.Usage.Input != 40 || muse.Usage.Calls != 2 || muse.Usage.WeeklyUsed == nil || *muse.Usage.WeeklyUsed != 29 {
		t.Fatalf("muse: %+v", muse.Usage)
	}
	var claude turnState
	parseClaude(`{"type":"result","result":"ok","num_turns":3,"usage":{"input_tokens":10,"cache_creation_input_tokens":20,"cache_read_input_tokens":70,"output_tokens":9}}`, &claude)
	if claude.Usage.Input != 100 || claude.Usage.Cached != 70 || claude.Usage.Calls != 3 {
		t.Fatalf("claude: %+v", claude.Usage)
	}
	var codex turnState
	parseCodex(`{"type":"turn.completed","usage":{"input_tokens":50,"cached_input_tokens":30,"output_tokens":7,"reasoning_output_tokens":2}}`, &codex)
	if codex.Usage.Input != 50 || codex.Usage.Cached != 30 || codex.Usage.Reasoning != 2 {
		t.Fatalf("codex: %+v", codex.Usage)
	}
}
