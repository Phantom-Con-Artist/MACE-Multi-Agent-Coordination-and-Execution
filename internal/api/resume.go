package api

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/url"
	"regexp"
	"sync"
	"time"
)

// A turn for an agent on another machine is one long request. Wi-Fi drops,
// sleep, and Tailscale reconnects would otherwise lose the agent's work or,
// if simply retried, run it twice. So each turn carries a key: the machine
// running it keeps going when the connection drops, and a retry with the
// same key joins the running turn or collects its result. Retries back off
// exponentially with full jitter, so machines returning together do not
// all ask at once.

// remoteRunKeep is how long a finished turn's result waits to be collected.
const remoteRunKeep = 30 * time.Minute

var turnKeyPattern = regexp.MustCompile(`^[0-9a-f]{16,64}$`)

type remoteRun struct {
	done   chan struct{}
	cancel context.CancelFunc
	res    turnResult
	err    error
}

type remoteRuns struct {
	mu   sync.Mutex
	runs map[string]*remoteRun
}

// join starts the turn under key unless it is already running or finished,
// then waits for it while ctx lasts. The turn itself outlives ctx.
func (rr *remoteRuns) join(ctx context.Context, key string, start func(context.Context) (turnResult, error)) (turnResult, error) {
	rr.mu.Lock()
	if rr.runs == nil {
		rr.runs = map[string]*remoteRun{}
	}
	run, ok := rr.runs[key]
	if !ok {
		runCtx, cancel := context.WithTimeout(context.Background(), turnLimit())
		run = &remoteRun{done: make(chan struct{}), cancel: cancel}
		rr.runs[key] = run
		go func() {
			defer cancel()
			run.res, run.err = start(runCtx)
			close(run.done)
			time.AfterFunc(remoteRunKeep, func() {
				rr.mu.Lock()
				if rr.runs[key] == run {
					delete(rr.runs, key)
				}
				rr.mu.Unlock()
			})
		}()
	}
	rr.mu.Unlock()
	select {
	case <-run.done:
		return run.res, run.err
	case <-ctx.Done():
		return turnResult{}, ctx.Err()
	}
}

// stop cancels the turn under key, when the machine that asked for it
// paused or stopped the conversation.
func (rr *remoteRuns) stop(key string) {
	rr.mu.Lock()
	run := rr.runs[key]
	rr.mu.Unlock()
	if run != nil {
		run.cancel()
	}
}

// resumable reports whether a joined machine keeps turns running across
// dropped connections.
func (h *APIHandler) resumable(ctx context.Context, p Peer) bool {
	if p.Node == "" {
		return false
	}
	var n Node
	return h.store.Document(ctx, "node", p.Node, &n) == nil && n.Protocol >= 3
}

// retryable reports whether a failed request may have lost only the
// connection. A machine that answered with an error has decided.
func retryable(err error) bool {
	var refused *nodeRefused
	return !errors.As(err, &refused) && !errors.Is(err, context.Canceled)
}

// backoff is full jitter: a random wait up to an exponentially growing cap.
func backoff(attempt int) time.Duration {
	const base, ceiling = time.Second, 30 * time.Second
	limit := ceiling
	if attempt < 5 {
		limit = min(ceiling, base<<attempt)
	}
	return rand.N(limit) + 100*time.Millisecond
}

// peerTurnResumable asks a joined machine for a turn and keeps asking with
// the same key until it answers, the turn limit passes, or ctx ends. When
// ctx ends the machine is told to stop the turn.
func (h *APIHandler) peerTurnResumable(ctx context.Context, p Peer, id string, body map[string]string) (turnResult, error) {
	path := "/sessions/" + url.PathEscape(id) + "/turn"
	body["key"] = newID()
	deadline := time.Now().Add(turnLimit())
	for attempt := 0; ; attempt++ {
		var out turnResult
		err := h.peerRequest(ctx, p, path, body, &out)
		if err == nil {
			return out, nil
		}
		if ctx.Err() == nil && retryable(err) && time.Now().Before(deadline) {
			select {
			case <-time.After(backoff(attempt)):
				continue
			case <-ctx.Done():
			}
		}
		if ctx.Err() != nil {
			// Best effort: the machine may be unreachable, and its turn then
			// ends at its own limit.
			stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			h.peerRequest(stopCtx, p, path, map[string]string{"key": body["key"], "cancel": "1"}, &struct{}{})
			cancel()
			return out, ctx.Err()
		}
		return out, err
	}
}
