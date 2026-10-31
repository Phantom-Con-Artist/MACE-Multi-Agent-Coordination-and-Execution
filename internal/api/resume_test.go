package api

import (
	"bytes"
	"context"
	"errors"
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
