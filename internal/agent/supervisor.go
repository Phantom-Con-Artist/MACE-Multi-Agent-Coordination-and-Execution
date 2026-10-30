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
	cancel       context.CancelFunc
	done         chan struct{}
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
	sem         chan struct{} // Concurrency limiter
	mu          sync.RWMutex
	agents      map[string]*managedAgent
	subMu       sync.RWMutex
	subscribers map[string][]chan *protocol.AgentEvent
}

// NewSupervisor creates a new agent supervisor with a concurrency limit.
func NewSupervisor(store *storage.Store, maxConcurrent int) *Supervisor {
	if maxConcurrent <= 0 {
		maxConcurrent = 10
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
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("invalid launch request: %w", err)
	}

	// Acquire concurrency slot
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrPoolCapacityFull
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

	// Prepare environment
	env := os.Environ()
	for k, v := range req.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	prepareCmdPlatform(cmd)

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
		process: proc,
		cmd:     cmd,
		stdin:   stdinPipe,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

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

	return managed.snapshot(), nil
}

func (s *Supervisor) pipeReader(r io.Reader, m *managedAgent, evtType protocol.EventType, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(r)
	// Support lines up to 1MB
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		text := scanner.Text()
		m.bytesRead.Add(int64(len(text) + 1))

		event := &protocol.AgentEvent{
			EventID:   generateID("evt_"),
			AgentID:   m.process.ID,
			Type:      evtType,
			Data:      text,
			Timestamp: time.Now().UTC(),
			Sequence:  m.seq.Add(1),
		}

		if s.store != nil {
			if err := s.store.AppendEvent(context.Background(), event); err != nil {
				fmt.Fprintf(os.Stderr, "failed to append event to store: %v\n", err)
			}
		}
		s.broadcastEvent(event)
	}
}

func (s *Supervisor) watchProcess(m *managedAgent, streamWg *sync.WaitGroup) {
	// Wait for stdout and stderr readers to finish reading to EOF before calling cmd.Wait()
	streamWg.Wait()
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
