package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"mace/internal/storage"
	"mace/pkg/protocol"
)

var (
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentNotRunning  = errors.New("agent is not running")
	ErrPoolCapacityFull = errors.New("maximum concurrent agent capacity reached")
)

type managedAgent struct {
	process      *protocol.AgentProcess
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdinMu      sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	done         chan struct{}
	lastOutput   atomic.Int64
	killReason   string
	onOutput     func(protocol.EventType, string, bool)
	pendingMu    sync.Mutex
	pending      []*protocol.AgentEvent
	seq          atomic.Uint64
	bytesRead    atomic.Int64
	bytesWritten atomic.Int64
	mu           sync.RWMutex
}

func (m *managedAgent) snapshot() *protocol.AgentProcess {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := m.process.Clone()
	cp.BytesRead = m.bytesRead.Load()
	cp.BytesWritten = m.bytesWritten.Load()
	return cp
}

// Supervisor coordinates concurrent AI agent lifecycles, I/O piping, and event broadcast.
type Supervisor struct {
	store       *storage.Store
	launchMu    sync.Mutex
	sem         chan struct{} // Concurrency limiter
	mu          sync.RWMutex
	agents      map[string]*managedAgent
	subMu       sync.RWMutex
	subscribers map[string][]chan *protocol.AgentEvent
	tag         string
}

// SetProcessTag marks every child with MACE_PROCESS_TAG so ReapOrphans can find
// processes left behind by a crashed daemon. Call it before launching anything.
func (s *Supervisor) SetProcessTag(tag string) { s.tag = tag }

// AcquireSlot waits for free capacity. Pass Reserved in the launch request to use it.
func (s *Supervisor) AcquireSlot(ctx context.Context) error {
	select {
	case s.sem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReleaseSlot returns a slot that was acquired but never handed to StartAgent.
func (s *Supervisor) ReleaseSlot() { <-s.sem }

// childEnv removes Mace credentials and daemon settings from the inherited
// environment. Agents, their tools, and desktop apps never need them.
func childEnv(extra map[string]string, tag string) []string {
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MACE_") {
			env = append(env, kv)
		}
	}
	for k, v := range extra {
		if !strings.HasPrefix(k, "MACE_") {
			env = append(env, k+"="+v)
		}
	}
	if tag != "" {
		env = append(env, "MACE_PROCESS_TAG="+tag)
	}
	return env
}

// NewSupervisor creates a new agent supervisor with a concurrency limit.
func NewSupervisor(store *storage.Store, maxConcurrent int) *Supervisor {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
	}
	// Process handles cannot survive a daemon restart. Reconcile old live records.
	if store != nil {
		if previous, err := store.ListAgents(context.Background()); err == nil {
			now := time.Now().UTC()
			for _, p := range previous {
				if !p.Status.IsTerminal() {
					_ = store.UpdateAgentStatus(context.Background(), p.ID, protocol.AgentStatusStopped, nil, "daemon restarted; process session unavailable", &now)
				}
			}
		}
	}
	return &Supervisor{
		store:       store,
		sem:         make(chan struct{}, maxConcurrent),
		agents:      make(map[string]*managedAgent),
		subscribers: make(map[string][]chan *protocol.AgentEvent),
	}
}

// StartAgent launches a new agent process under supervised execution.
func (s *Supervisor) StartAgent(ctx context.Context, req protocol.AgentLaunchRequest) (*protocol.AgentProcess, error) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if req.ID != "" {
		if prior, _ := s.GetAgent(req.ID); prior != nil {
			if req.Reserved {
				<-s.sem
			}
			return nil, fmt.Errorf("agent ID already exists")
		}
	}
	if err := req.Validate(); err != nil {
		if req.Reserved {
			<-s.sem
		}
		return nil, fmt.Errorf("invalid launch request: %w", err)
	}

	// Acquire concurrency slot
	if !req.Reserved {
		select {
		case s.sem <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, ErrPoolCapacityFull
		}
	}

	agentID := req.ID
	if agentID == "" {
		agentID = generateID("ag_")
	}

	workingDir := req.WorkingDir
	if workingDir == "" {
		workingDir, _ = os.Getwd()
	}

	agentCtx, cancel := context.WithCancel(context.Background())
	if req.TimeoutSeconds > 0 {
		agentCtx, cancel = context.WithTimeout(context.Background(), time.Duration(req.TimeoutSeconds)*time.Second)
	}

	cmd := exec.CommandContext(agentCtx, req.Command, req.Args...)
	cmd.Dir = workingDir

	cmd.Env = childEnv(req.Env, s.tag)
	prepareCmdPlatform(cmd)
	cmd.Cancel = func() error { return killProcessPlatform(cmd, syscall.SIGKILL) }

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		<-s.sem
		return nil, fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		stdinPipe.Close()
		cancel()
		<-s.sem
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		cancel()
		<-s.sem
		return nil, fmt.Errorf("failed to open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdinPipe.Close()
		stdoutPipe.Close()
		stderrPipe.Close()
		cancel()
		<-s.sem
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	proc := &protocol.AgentProcess{
		ID:         agentID,
		Name:       req.Name,
		PID:        cmd.Process.Pid,
		Status:     protocol.AgentStatusRunning,
		Command:    req.Command,
		Args:       req.Args,
		WorkingDir: workingDir,
		StartedAt:  time.Now().UTC(),
		Labels:     req.Labels,
	}

	managed := &managedAgent{
		process:  proc,
		cmd:      cmd,
		stdin:    stdinPipe,
		ctx:      agentCtx,
		cancel:   cancel,
		done:     make(chan struct{}),
		onOutput: req.OnOutput,
	}
	managed.lastOutput.Store(time.Now().UnixNano())

	s.mu.Lock()
	s.agents[agentID] = managed
	s.mu.Unlock()

	// Persist to storage
	if s.store != nil {
		_ = s.store.SaveAgent(context.Background(), proc)
		_ = s.store.AppendAudit(context.Background(), "supervisor", "START_AGENT", agentID, map[string]interface{}{
			"pid":     proc.PID,
			"command": proc.Command,
			"args":    proc.Args,
		})
	}

	// Broadcast initial running status event
	s.broadcastEvent(&protocol.AgentEvent{
		EventID:   generateID("evt_"),
		AgentID:   agentID,
		Type:      protocol.EventStatus,
		Data:      string(protocol.AgentStatusRunning),
		Timestamp: time.Now().UTC(),
		Sequence:  managed.seq.Add(1),
	})

	// Spawn stream reader goroutines
	var streamWg sync.WaitGroup
	streamWg.Add(2)
	go s.pipeReader(stdoutPipe, managed, protocol.EventStdout, &streamWg)
	go s.pipeReader(stderrPipe, managed, protocol.EventStderr, &streamWg)

	// Spawn process watcher
	go s.watchProcess(managed, &streamWg)
	go s.flushLoop(managed)
	if req.IdleTimeoutSeconds > 0 {
		go s.watchIdle(managed, time.Duration(req.IdleTimeoutSeconds)*time.Second)
	}

	return managed.snapshot(), nil
}

// maxEventChunk bounds one stored event. Longer lines are split into chunks
// marked partial, so the pipe is always drained and the child never blocks.
const maxEventChunk = 1 << 20

func (s *Supervisor) pipeReader(r io.Reader, m *managedAgent, evtType protocol.EventType, wg *sync.WaitGroup) {
	defer wg.Done()
	reader := bufio.NewReaderSize(r, 64*1024)
	var line []byte
	for {
		chunk, isPrefix, err := reader.ReadLine()
		line = append(line, chunk...)
		if err != nil {
			if len(line) > 0 {
				s.emitOutput(m, evtType, string(line), false)
			}
			if err != io.EOF {
				s.emitOutput(m, protocol.EventStderr, "[mace] output reader error: "+err.Error(), false)
				_, _ = io.Copy(io.Discard, r)
			}
			return
		}
		if isPrefix && len(line) < maxEventChunk {
			continue
		}
		s.emitOutput(m, evtType, string(line), isPrefix)
		line = line[:0]
	}
}

func (s *Supervisor) emitOutput(m *managedAgent, evtType protocol.EventType, text string, partial bool) {
	m.lastOutput.Store(time.Now().UnixNano())
	if !partial {
		m.bytesRead.Add(int64(len(text) + 1))
	} else {
		m.bytesRead.Add(int64(len(text)))
	}
	event := &protocol.AgentEvent{
		EventID:   generateID("evt_"),
		AgentID:   m.process.ID,
		Type:      evtType,
		Data:      text,
		Timestamp: time.Now().UTC(),
		Sequence:  m.seq.Add(1),
	}
	if partial {
		event.Metadata = map[string]string{"partial": "true"}
	}
	if m.onOutput != nil {
		m.onOutput(evtType, text, partial)
	}
	if s.store != nil {
		m.pendingMu.Lock()
		m.pending = append(m.pending, event)
		full := len(m.pending) >= 500
		m.pendingMu.Unlock()
		if full {
			s.flush(m)
		}
	}
	s.broadcastEvent(event)
}

// Output is written in batches: one transaction per 100 ms or 500 lines,
// instead of one per line, so parallel agents do not queue on the writer.
func (s *Supervisor) flush(m *managedAgent) {
	m.pendingMu.Lock()
	batch := m.pending
	m.pending = nil
	m.pendingMu.Unlock()
	if s.store == nil || len(batch) == 0 {
		return
	}
	if err := s.store.AppendEvents(context.Background(), batch); err != nil {
		fmt.Fprintf(os.Stderr, "failed to append events to store: %v\n", err)
	}
}

func (s *Supervisor) flushLoop(m *managedAgent) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			s.flush(m)
		}
	}
}

// watchIdle stops a process that has produced no output for limit.
func (s *Supervisor) watchIdle(m *managedAgent, limit time.Duration) {
	interval := min(limit/4, 5*time.Second)
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			if time.Since(time.Unix(0, m.lastOutput.Load())) < limit {
				continue
			}
			m.mu.Lock()
			m.killReason = fmt.Sprintf("no output for %s", limit.Round(time.Second))
			m.mu.Unlock()
			_ = s.StopAgent(m.process.ID, "SIGTERM", 3*time.Second)
			return
		}
	}
}

func (s *Supervisor) watchProcess(m *managedAgent, streamWg *sync.WaitGroup) {
	defer m.cancel()
	// Wait for stdout and stderr readers to finish reading to EOF before calling cmd.Wait()
	streamWg.Wait()
	// All output is stored before the process is reported finished, so readers
	// that wait for a terminal status always see complete output.
	s.flush(m)
	err := m.cmd.Wait()

	stoppedAt := time.Now().UTC()
	var exitCode int
	var status protocol.AgentStatus
	var errMsg string

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
			m.mu.RLock()
			isKilled := m.process.Status == protocol.AgentStatusKilled
			m.mu.RUnlock()
			if exitCode == -1 || isKilled {
				status = protocol.AgentStatusKilled
			} else {
				status = protocol.AgentStatusFailed
			}
		} else {
			exitCode = 1
			status = protocol.AgentStatusFailed
			errMsg = err.Error()
		}
	} else {
		exitCode = 0
		status = protocol.AgentStatusStopped
	}

	m.stdinMu.Lock()
	if m.stdin != nil {
		_ = m.stdin.Close()
	}
	m.stdinMu.Unlock()

	m.mu.Lock()
	if errMsg == "" && m.killReason != "" {
		errMsg = m.killReason
	}
	if errMsg == "" && errors.Is(m.ctx.Err(), context.DeadlineExceeded) {
		errMsg = "time limit reached"
	}
	m.process.Status = status
	m.process.ExitCode = &exitCode
	m.process.StoppedAt = &stoppedAt
	m.process.Error = errMsg
	m.process.BytesRead = m.bytesRead.Load()
	m.process.BytesWritten = m.bytesWritten.Load()
	m.mu.Unlock()

	// Release concurrency slot
	<-s.sem
	close(m.done)

	// Persist terminal state
	if s.store != nil {
		_ = s.store.UpdateAgentStatus(context.Background(), m.process.ID, status, &exitCode, errMsg, &stoppedAt)
		_ = s.store.AppendAudit(context.Background(), "supervisor", "PROCESS_EXIT", m.process.ID, map[string]interface{}{
			"exit_code": exitCode,
			"status":    status,
			"error":     errMsg,
		})
	}

	// Broadcast exit event
	s.broadcastEvent(&protocol.AgentEvent{
		EventID:   generateID("evt_"),
		AgentID:   m.process.ID,
		Type:      protocol.EventExit,
		Data:      fmt.Sprintf("%d", exitCode),
		Timestamp: stoppedAt,
		Sequence:  m.seq.Add(1),
		Metadata: map[string]string{
			"status": string(status),
		},
	})
}

// SendInput writes data to the agent process stdin.
func (s *Supervisor) CloseInput(id string) error {
	s.mu.RLock()
	m := s.agents[id]
	s.mu.RUnlock()
	if m == nil {
		return ErrAgentNotFound
	}
	m.stdinMu.Lock()
	defer m.stdinMu.Unlock()
	return m.stdin.Close()
}

func (s *Supervisor) SendInput(agentID string, data string) error {
	s.mu.RLock()
	managed, exists := s.agents[agentID]
	s.mu.RUnlock()

	if !exists {
		return ErrAgentNotFound
	}

	managed.mu.RLock()
	isRunning := managed.process.Status == protocol.AgentStatusRunning
	managed.mu.RUnlock()
	if !isRunning {
		return ErrAgentNotRunning
	}

	managed.stdinMu.Lock()
	defer managed.stdinMu.Unlock()

	n, err := io.WriteString(managed.stdin, data)
	if err != nil {
		return fmt.Errorf("failed to write to stdin: %w", err)
	}
	managed.bytesWritten.Add(int64(n))
	return nil
}

// StopAgent terminates an agent with a graceful signal and fallback SIGKILL timeout.
func (s *Supervisor) StopAgent(agentID string, signalName string, graceTimeout time.Duration) error {
	s.mu.RLock()
	managed, exists := s.agents[agentID]
	s.mu.RUnlock()

	if !exists {
		return ErrAgentNotFound
	}

	managed.mu.Lock()
	if managed.process.Status.IsTerminal() {
		managed.mu.Unlock()
		return nil
	}
	managed.process.Status = protocol.AgentStatusKilled
	managed.mu.Unlock()

	sig := syscall.SIGTERM
	if signalName == "SIGINT" {
		sig = syscall.SIGINT
	} else if signalName == "SIGKILL" {
		sig = syscall.SIGKILL
	}

	if graceTimeout <= 0 {
		graceTimeout = 3 * time.Second
	}

	// Send initial signal
	_ = killProcessPlatform(managed.cmd, sig)

	// Wait with timeout
	select {
	case <-managed.done:
		return nil
	case <-time.After(graceTimeout):
		// Force kill
		_ = killProcessPlatform(managed.cmd, syscall.SIGKILL)
		managed.cancel()
		<-managed.done
		return nil
	}
}

// GetAgent retrieves agent details from memory or fallback store.
func (s *Supervisor) GetAgent(id string) (*protocol.AgentProcess, error) {
	s.mu.RLock()
	managed, exists := s.agents[id]
	if exists {
		snap := managed.snapshot()
		s.mu.RUnlock()
		return snap, nil
	}
	s.mu.RUnlock()

	if s.store != nil {
		return s.store.GetAgent(context.Background(), id)
	}

	return nil, ErrAgentNotFound
}

// ListAgents returns all active and stored agents.
func (s *Supervisor) ListAgents(ctx context.Context) ([]*protocol.AgentProcess, error) {
	if s.store != nil {
		return s.store.ListAgents(ctx)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*protocol.AgentProcess, 0, len(s.agents))
	for _, m := range s.agents {
		res = append(res, m.snapshot())
	}
	return res, nil
}

// Subscribe returns an event channel for real-time agent output streaming.
func (s *Supervisor) Subscribe(agentID string) (<-chan *protocol.AgentEvent, func()) {
	ch := make(chan *protocol.AgentEvent, 100)

	s.subMu.Lock()
	s.subscribers[agentID] = append(s.subscribers[agentID], ch)
	s.subMu.Unlock()

	unsubscribe := func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()

		subs := s.subscribers[agentID]
		for i, sub := range subs {
			if sub == ch {
				s.subscribers[agentID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return ch, unsubscribe
}

func (s *Supervisor) broadcastEvent(event *protocol.AgentEvent) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()

	subs := s.subscribers[event.AgentID]
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Non-blocking drop to prevent slow consumers from hanging the supervisor
		}
	}
}

// Close stops all running agents and cleans up resources.
func (s *Supervisor) Close() error {
	s.mu.RLock()
	var active []*managedAgent
	for _, m := range s.agents {
		if !m.process.Status.IsTerminal() {
			active = append(active, m)
		}
	}
	s.mu.RUnlock()

	for _, m := range active {
		_ = s.StopAgent(m.process.ID, "SIGTERM", 1*time.Second)
	}
	return nil
}

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}
