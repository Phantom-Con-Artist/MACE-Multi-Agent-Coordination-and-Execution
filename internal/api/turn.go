package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"mace/pkg/protocol"
)

// turnResult is what one agent turn produced.
type turnResult struct {
	Text     string     `json:"text"`
	Activity []Activity `json:"activity,omitempty"`
	// Reset means a resume was requested but the CLI started a new session,
	// so the agent lost its native memory of earlier turns.
	Reset bool `json:"reset,omitempty"`
	// Usage is what the turn cost, when the CLI reports it.
	Usage *Usage `json:"usage,omitempty"`
}

// lockTurn waits until no other turn of this agent is running. Inbox turns and
// turns requested by connected hosts share the agent's one native session.
func (h *APIHandler) lockTurn(ctx context.Context, id string) (func(), error) {
	v, _ := h.turnLocks.LoadOrStore(id, make(chan struct{}, 1))
	lock := v.(chan struct{})
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// turn runs one native CLI invocation in the agent's native session, resuming
// it when possible, and streams tool activity to live while it works.
//
// origin is "local" for conversations run by this host and "peer" for turns a
// connected host requested. The presence guard only stops local work; peer turns
// stop when the requesting host cancels or its connection drops.
func (h *APIHandler) turn(ctx context.Context, id string, p turnPrompt, origin string, live func([]Activity)) (turnResult, error) {
	unlock, err := h.lockTurn(ctx, id)
	if err != nil {
		return turnResult{}, err
	}
	defer unlock()
	return h.runTurn(ctx, id, p, origin, live, true)
}

func (h *APIHandler) runTurn(ctx context.Context, id string, p turnPrompt, origin string, live func([]Activity), allowRetry bool) (turnResult, error) {
	// Wait for capacity before taking the lifecycle lock, so stop and terminate
	// stay responsive while many agents are queued.
	if e := h.supervisor.AcquireSlot(ctx); e != nil {
		return turnResult{}, e
	}
	reserved := true
	defer func() {
		if reserved {
			h.supervisor.ReleaseSlot()
		}
	}()
	h.lifecycleMu.Lock()
	locked := true
	defer func() {
		if locked {
			h.lifecycleMu.Unlock()
		}
	}()
	var s Session
	if e := h.store.Document(ctx, "session", id, &s); e != nil {
		return turnResult{}, fmt.Errorf("agent session unavailable: %w", e)
	}
	if s.Status == "stopped" || s.Status == "terminated" || s.Status == "removed" {
		return turnResult{}, fmt.Errorf("agent is %s; resume it before chatting", s.Status)
	}
	if err := ctx.Err(); err != nil {
		return turnResult{}, err
	}
	ad, ok := adapters[s.Provider]
	if !ok || (s.Provider == "diagnostic" && os.Getenv("MACE_TEST_MODE") != "1") {
		return turnResult{}, errors.New("unsupported agent adapter")
	}
	resume := s.NativeSession != "" && s.NativeDir == s.Directory
	nativeID := s.NativeSession
	if !resume {
		nativeID = ""
		if ad.chooseID {
			nativeID = newUUID()
		}
	}
	prompt := p.Full
	if resume && p.Delta != "" {
		prompt = p.Delta
		if slim := h.slimHeader(ctx, s, p); slim != "" && p.Body != "" {
			prompt = slim + p.Body
		}
	}
	if !resume {
		prompt = agentInstructions(s) + "\n" + prompt
	}
	aware := h.awareness(ctx, s)
	awareHash := awarenessHash(aware)
	if resume {
		aware = h.compactAwareness(ctx, s, aware, awareHash)
	}
	prompt = aware + "\n" + prompt
	command, args := s.Provider, ad.args(s, prompt, nativeID, resume)
	if s.Provider == "diagnostic" {
		command = "cat"
	}

	// Output is parsed as it arrives, so the conversation shows tool steps live.
	state := &turnState{}
	var mu sync.Mutex
	var line strings.Builder
	lastPublish := time.Time{}
	publish := func(force bool) {
		if live == nil || (!state.changed && !force) || (!force && time.Since(lastPublish) < 150*time.Millisecond) {
			return
		}
		state.changed, lastPublish = false, time.Now()
		live(append([]Activity(nil), state.Steps...))
	}
	launch := protocol.AgentLaunchRequest{
		Name: s.Name + " reply", Command: command, Args: args, WorkingDir: s.Directory,
		TimeoutSeconds: int(turnLimit().Seconds()), Reserved: true, Env: h.contextEnv(id, p.Room),
		Labels: map[string]string{"session": id, "origin": origin},
		OnOutput: func(stream protocol.EventType, text string, partial bool) {
			if stream != protocol.EventStdout {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			line.WriteString(text)
			if partial {
				return
			}
			ad.parse(line.String(), state)
			line.Reset()
			publish(false)
		},
	}
	if ad.streams {
		launch.IdleTimeoutSeconds = int(idleLimit().Seconds())
	}
	h.updateSession(context.Background(), id, func(s *Session) { s.Status = "replying" })
	defer h.updateSession(context.Background(), id, func(s *Session) {
		if s.Status == "replying" {
			s.Status = "ready"
		}
	})
	reserved = false
	proc, e := h.supervisor.StartAgent(ctx, launch)
	if e != nil {
		return turnResult{}, e
	}
	h.audit(id, "TURN_STARTED", proc.ID, map[string]any{"session": id, "resume": resume, "prompt_bytes": len(prompt)})
	if s.Provider == "diagnostic" {
		// Each reply differs, as real replies do, so loop detection stays quiet.
		h.supervisor.SendInput(proc.ID, "Diagnostic reply "+proc.ID+". [GOAL_COMPLETE]\n")
	}
	h.supervisor.CloseInput(proc.ID)
	h.lifecycleMu.Unlock()
	locked = false

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var finished *protocol.AgentProcess
	for finished == nil {
		select {
		case <-ctx.Done():
			h.supervisor.StopAgent(proc.ID, "SIGTERM", time.Second)
			return turnResult{}, ctx.Err()
		case <-ticker.C:
			p, e := h.supervisor.GetAgent(proc.ID)
			if e != nil {
				return turnResult{}, e
			}
			if p.Status.IsTerminal() {
				finished = p
			}
		}
	}
	mu.Lock()
	if line.Len() > 0 {
		ad.parse(line.String(), state)
	}
	publish(true)
	mu.Unlock()
	_, stderrLines, _ := h.processOutput(ctx, proc.ID)
	stderr := strings.Join(stderrLines, "\n")

	failed := finished.ExitCode != nil && *finished.ExitCode != 0
	if failed && resume && allowRetry && ad.resumeMissing != nil && ad.resumeMissing(stderr) {
		// The native session is gone; start a fresh one with the full context.
		h.updateSession(context.Background(), id, func(s *Session) { s.NativeSession, s.NativeDir = "", "" })
		res, e := h.runTurn(ctx, id, p, origin, live, false)
		res.Reset = true
		return res, e
	}
	if failed {
		if finished.Error != "" {
			return turnResult{}, fmt.Errorf("%s was stopped: %s", s.Name, finished.Error)
		}
		return turnResult{}, fmt.Errorf("%s exited (%d): %s", s.Name, *finished.ExitCode, truncate(stderr, 3000))
	}
	if state.Err != "" {
		return turnResult{}, errors.New(state.Err)
	}
	reply := state.reply()
	if reply == "" {
		return turnResult{}, errors.New("agent returned no response; check host activity and provider login")
	}
	reported := state.SessionID
	if reported == "" {
		reported = nativeID
	}
	if reported != "" && s.Provider != "diagnostic" {
		dir := s.Directory
		h.updateSession(context.Background(), id, func(s *Session) { s.NativeSession, s.NativeDir = reported, dir })
		h.store.PutDocument(context.Background(), "briefing", "aware|"+id, briefing{Hash: awareHash, Native: reported})
		// The session now holds this header, unless it was replaced by a
		// fresh one that saw only part of it.
		if p.Brief != "" && p.Room != "" && (!resume || reported == nativeID) {
			h.store.PutDocument(context.Background(), "briefing", id+"|"+p.Room, briefing{Hash: p.Brief, Native: reported, Team: p.Team})
		}
	}
	res := turnResult{Text: reply, Activity: state.Steps, Reset: resume && reported != nativeID}
	details := map[string]any{"session": id, "steps": len(state.Steps)}
	if !state.Usage.empty() {
		res.Usage = &state.Usage
		details["usage"] = state.Usage
	}
	h.audit(id, "TURN_COMPLETED", proc.ID, details)
	return res, nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// remoteTurn runs a turn requested by a connected host. Older hosts send a
// single prompt; current hosts send full and delta forms.
func (h *APIHandler) remoteTurn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
		Key    string `json:"key"`
		Cancel string `json:"cancel"`
		turnPrompt
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req) != nil || (req.Key != "" && !turnKeyPattern.MatchString(req.Key)) {
		http.Error(w, "invalid turn", 400)
		return
	}
	id := routeParam(r, "id")
	// Keys are scoped to the machine that asked, so one machine cannot
	// collect or stop another's turn.
	key := r.Header.Get("X-Mace-Node") + "|" + id + "|" + req.Key
	if req.Cancel != "" && req.Key != "" {
		h.remoteRuns.stop(key)
		writeJSON(w, 200, map[string]any{})
		return
	}
	if req.Prompt == "" && req.Full == "" {
		http.Error(w, "prompt required", 400)
		return
	}
	if req.Full == "" {
		req.Full = req.Prompt
	}
	var res turnResult
	var e error
	if req.Key != "" {
		res, e = h.remoteRuns.join(r.Context(), key, func(ctx context.Context) (turnResult, error) {
			return h.turn(ctx, id, req.turnPrompt, "peer", nil)
		})
	} else {
		ctx, cancel := context.WithTimeout(r.Context(), turnLimit())
		defer cancel()
		res, e = h.turn(ctx, id, req.turnPrompt, "peer", nil)
	}
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, 200, res)
}

var latestLine = regexp.MustCompile(` Latest: .*`)

func awarenessHash(aware string) string {
	sum := sha256.Sum256([]byte(latestLine.ReplaceAllString(aware, "")))
	return hex.EncodeToString(sum[:12])
}

// compactAwareness shortens the awareness block for a resumed session that
// already holds it: only the first line is sent again unless the agent's
// identity, role, or conversations changed. Latest-message previews alone
// do not count as a change.
func (h *APIHandler) compactAwareness(ctx context.Context, s Session, aware, hash string) string {
	var b briefing
	if h.store.Document(ctx, "briefing", "aware|"+s.ID, &b) == nil && b.Hash == hash && b.Native == s.NativeSession {
		first, _, _ := strings.Cut(aware, "\n")
		return first + "\n"
	}
	return aware
}
