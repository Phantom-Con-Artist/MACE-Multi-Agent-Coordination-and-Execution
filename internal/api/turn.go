package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	}
	if !resume {
		prompt = agentInstructions(s) + "\n" + prompt
	}
	prompt = h.awareness(ctx, s) + "\n" + prompt
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
		TimeoutSeconds: int(turnLimit().Seconds()), Reserved: true,
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
	h.audit(id, "TURN_STARTED", proc.ID, map[string]any{"session": id, "resume": resume})
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
	}
	h.audit(id, "TURN_COMPLETED", proc.ID, map[string]any{"session": id, "steps": len(state.Steps)})
	return turnResult{Text: reply, Activity: state.Steps, Reset: resume && reported != nativeID}, nil
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
		turnPrompt
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req) != nil || (req.Prompt == "" && req.Full == "") {
		http.Error(w, "prompt required", 400)
		return
	}
	if req.Full == "" {
		req.Full = req.Prompt
	}
	ctx, cancel := context.WithTimeout(r.Context(), turnLimit())
	defer cancel()
	res, e := h.turn(ctx, routeParam(r, "id"), req.turnPrompt, "peer", nil)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, 200, res)
}
