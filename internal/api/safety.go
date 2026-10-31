package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"mace/pkg/protocol"
)

// Causes recorded when a conversation is cancelled. They decide whether the run
// is reported as paused by the owner or interrupted and offered Continue.
var (
	errShutdown  = errors.New("host shutting down")
	errOwnerAway = errors.New("owner disconnected")
	errPausedAll = errors.New("owner paused everything")
)

func interruption(cause error) string {
	switch {
	case errors.Is(cause, errOwnerAway):
		return "Connection to your device was lost, so agents were stopped. Continue when you are back."
	case errors.Is(cause, errPausedAll):
		return "Paused with Pause everything. Continue when you are ready."
	case errors.Is(cause, errShutdown):
		return "The host shut down while agents were working. Continue when you are ready."
	}
	return ""
}

func envMinutes(key string, fallback int) time.Duration {
	if v, e := strconv.Atoi(os.Getenv(key)); e == nil && v > 0 {
		return time.Duration(v) * time.Minute
	}
	return time.Duration(fallback) * time.Minute
}

// turnLimit is the hard cap for one agent turn. idleLimit stops streaming
// adapters that report nothing for that long.
func turnLimit() time.Duration { return envMinutes("MACE_TURN_LIMIT_MINUTES", 120) }
func idleLimit() time.Duration { return envMinutes("MACE_TURN_IDLE_MINUTES", 15) }

// peerTransport notices a vanished peer in about 25 seconds instead of minutes.
var peerTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:         10 * time.Second,
		KeepAliveConfig: net.KeepAliveConfig{Enable: true, Idle: 10 * time.Second, Interval: 5 * time.Second, Count: 3},
	}).DialContext,
	TLSHandshakeTimeout: 10 * time.Second,
	IdleConnTimeout:     90 * time.Second,
}

// processOutput returns complete stdout and stderr lines of a finished process,
// joining chunks the supervisor split from oversized lines.
func (h *APIHandler) processOutput(ctx context.Context, id string) (stdout, stderr []string, err error) {
	var pending = map[protocol.EventType][]byte{}
	after := uint64(0)
	for {
		events, e := h.store.GetAgentEvents(ctx, id, after, 5000)
		if e != nil {
			return nil, nil, e
		}
		for _, ev := range events {
			after = ev.Sequence
			if ev.Type != protocol.EventStdout && ev.Type != protocol.EventStderr {
				continue
			}
			pending[ev.Type] = append(pending[ev.Type], ev.Data...)
			if ev.Metadata["partial"] == "true" {
				continue
			}
			line := string(pending[ev.Type])
			pending[ev.Type] = pending[ev.Type][:0]
			if ev.Type == protocol.EventStdout {
				stdout = append(stdout, line)
			} else {
				stderr = append(stderr, line)
			}
		}
		if len(events) < 5000 {
			return stdout, stderr, nil
		}
	}
}

// stopWork cancels conversations and their native processes. With localOnly,
// turns that a connected host requested are left to that host to manage.
func (h *APIHandler) stopWork(cause error, localOnly bool) (runs, processes int) {
	runs = h.stopAllRuns(cause)
	list, _ := h.supervisor.ListAgents(context.Background())
	var wg sync.WaitGroup
	for _, p := range list {
		if p.Status.IsTerminal() || p.Labels["session"] == "" || (localOnly && p.Labels["origin"] == "peer") {
			continue
		}
		processes++
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			h.supervisor.StopAgent(id, "SIGTERM", 2*time.Second)
		}(p.ID)
	}
	wg.Wait()
	return runs, processes
}

// pauseAll stops every conversation and agent process on this host, and asks
// connected hosts to do the same unless this request was forwarded by one.
func (h *APIHandler) pauseAll(w http.ResponseWriter, r *http.Request) {
	runs, processes := h.stopWork(errPausedAll, false)
	forwarded := r.URL.Query().Get("forwarded") == "1"
	peers := map[string]string{}
	if !forwarded {
		raw, _ := h.store.Documents(r.Context(), "peer")
		var wg sync.WaitGroup
		var mu sync.Mutex
		for _, b := range raw {
			var p Peer
			if json.Unmarshal(b, &p) != nil {
				continue
			}
			wg.Add(1)
			go func(p Peer) {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				var out map[string]any
				result := "paused"
				if e := h.peerRequest(ctx, p, "/pause-all?forwarded=1", map[string]bool{}, &out); e != nil {
					result = e.Error()
				}
				mu.Lock()
				peers[p.ID] = result
				mu.Unlock()
			}(p)
		}
		wg.Wait()
	}
	h.audit("you", "PAUSE_ALL", "", map[string]any{"runs": runs, "processes": processes, "peers": peers, "forwarded": forwarded})
	writeJSON(w, 200, map[string]any{"runs": runs, "processes": processes, "peers": peers})
}

// continueConversation resumes an interrupted or paused conversation.
func (h *APIHandler) continueConversation(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "room")
	if _, _, e := h.conversation(r.Context(), id); e != nil {
		code := 404
		if e == errRemoved {
			code = http.StatusGone
		}
		http.Error(w, e.Error(), code)
		return
	}
	m, e := h.wakeAll(r.Context(), id, "Continued after the interruption. Agents pick up where they stopped and check what was already done before repeating anything.")
	if e != nil {
		http.Error(w, e.Error(), 409)
		return
	}
	h.audit("you", "CONVERSATION_CONTINUED", id, nil)
	writeJSON(w, 201, m)
}

// Presence guard. Signed-in browsers ping every few seconds. When the last
// device has been silent for presenceTimeout plus the grace period, local agent
// work stops. The guard is only armed after a device has been present, so work
// started through the API with no browser open is not affected.
const presenceTimeout = 10 * time.Second

type presence struct {
	mu       sync.Mutex
	lastSeen map[string]time.Time
	armed    bool
	stopped  time.Time
}

func presenceGrace() (time.Duration, bool) {
	v := os.Getenv("MACE_PRESENCE_GRACE_SECONDS")
	if v == "off" {
		return 0, false
	}
	if n, e := strconv.Atoi(v); e == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	// A phone browser goes quiet as soon as the screen locks, so the default
	// gives the owner ten minutes before agents stop.
	return 10 * time.Minute, true
}

// presenceSetting is the owner's choice about working while away.
type presenceSetting struct {
	KeepWorking bool `json:"keep_working"`
}

func (h *APIHandler) keepWorking() bool {
	var s presenceSetting
	return h.store != nil && h.store.Document(context.Background(), "setting", "presence", &s) == nil && s.KeepWorking
}

// presenceSettings serves GET and POST /api/v1/settings/presence.
func (h *APIHandler) presenceSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var s presenceSetting
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&s) != nil {
			http.Error(w, "Send keep_working as true or false.", 400)
			return
		}
		if err := h.store.PutDocument(r.Context(), "setting", "presence", s); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		h.audit("you", "PRESENCE_SETTING_CHANGED", "", map[string]any{"keep_working": s.KeepWorking})
	}
	grace, enabled := presenceGrace()
	writeJSON(w, 200, map[string]any{"keep_working": h.keepWorking(), "guard": enabled, "grace_seconds": int(grace.Seconds())})
}

func (h *APIHandler) presencePing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	if req.Device == "" || len(req.Device) > 100 {
		http.Error(w, "device required", 400)
		return
	}
	h.presence.mu.Lock()
	h.presence.lastSeen[req.Device] = time.Now()
	h.presence.armed = true
	stopped := h.presence.stopped
	h.presence.mu.Unlock()
	grace, enabled := presenceGrace()
	out := map[string]any{"guard": enabled, "grace_seconds": int(grace.Seconds()), "timeout_seconds": int(presenceTimeout.Seconds())}
	if !stopped.IsZero() {
		out["stopped_at"] = stopped
	}
	writeJSON(w, 200, out)
}

// reloadSlack keeps a page reload, which leaves and pings again within a second,
// from counting as departure when the grace period is zero.
const reloadSlack = 3 * time.Second

// presenceLeave lets a closing page report departure without waiting for the
// ping timeout.
func (h *APIHandler) presenceLeave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Device string `json:"device"`
	}
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&req)
	h.presence.mu.Lock()
	if _, ok := h.presence.lastSeen[req.Device]; ok {
		h.presence.lastSeen[req.Device] = time.Now().Add(-presenceTimeout + reloadSlack)
	}
	h.presence.mu.Unlock()
	w.WriteHeader(204)
}

// ownerAway reports whether the guard should stop work now.
func (h *APIHandler) ownerAway(now time.Time) bool {
	grace, enabled := presenceGrace()
	h.presence.mu.Lock()
	defer h.presence.mu.Unlock()
	if !enabled || !h.presence.armed || h.keepWorking() {
		return false
	}
	for device, seen := range h.presence.lastSeen {
		if now.Sub(seen) < presenceTimeout+grace {
			return false
		}
		delete(h.presence.lastSeen, device)
	}
	h.presence.armed = false
	return true
}

func (h *APIHandler) guardPresence(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if !h.ownerAway(now) {
				continue
			}
			if runs, processes := h.stopWork(errOwnerAway, true); runs+processes > 0 {
				h.presence.mu.Lock()
				h.presence.stopped = now.UTC()
				h.presence.mu.Unlock()
				h.audit("mace", "OWNER_DISCONNECTED", "", map[string]any{"runs": runs, "processes": processes})
			}
		}
	}
}
