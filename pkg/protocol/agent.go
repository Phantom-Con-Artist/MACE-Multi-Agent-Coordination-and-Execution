package protocol

import (
	"errors"
	"strings"
	"time"
)

// AgentStatus represents the lifecycle state of a managed agent process.
type AgentStatus string

const (
	AgentStatusPending  AgentStatus = "pending"
	AgentStatusStarting AgentStatus = "starting"
	AgentStatusRunning  AgentStatus = "running"
	AgentStatusStopped  AgentStatus = "stopped"
	AgentStatusFailed   AgentStatus = "failed"
	AgentStatusKilled   AgentStatus = "killed"
)

// IsTerminal returns true if the status indicates the agent process has ended.
func (s AgentStatus) IsTerminal() bool {
	switch s {
	case AgentStatusStopped, AgentStatusFailed, AgentStatusKilled:
		return true
	default:
		return false
	}
}

// AgentLaunchRequest defines the parameters required to spawn a new agent.
type AgentLaunchRequest struct {
	ID             string            `json:"id,omitempty"`
	Name           string            `json:"name"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Interactive    bool              `json:"interactive,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	// IdleTimeoutSeconds stops a process that produces no output for this long.
	// Only use it for adapters that stream progress while working.
	IdleTimeoutSeconds int               `json:"idle_timeout_seconds,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	// Reserved means the caller already holds a slot from AcquireSlot. The
	// supervisor takes ownership of that slot whether or not the launch succeeds.
	Reserved bool `json:"-"`
	// OnOutput receives every output line synchronously, in order, before it is
	// stored. Long lines arrive in chunks with partial set. It must not block.
	OnOutput func(stream EventType, text string, partial bool) `json:"-"`
}

// Validate checks whether the launch request has valid parameters.
func (r *AgentLaunchRequest) Validate() error {
	if strings.TrimSpace(r.Command) == "" {
		return errors.New("agent command cannot be empty")
	}
	if strings.TrimSpace(r.Name) == "" {
		r.Name = r.Command
	}
	if r.TimeoutSeconds < 0 || r.IdleTimeoutSeconds < 0 {
		return errors.New("timeout_seconds cannot be negative")
	}
	return nil
}

// AgentProcess represents the runtime metadata of a managed agent process.
type AgentProcess struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	PID          int               `json:"pid"`
	Status       AgentStatus       `json:"status"`
	Command      string            `json:"command"`
	Args         []string          `json:"args,omitempty"`
	WorkingDir   string            `json:"working_dir"`
	ExitCode     *int              `json:"exit_code,omitempty"`
	Error        string            `json:"error,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	StoppedAt    *time.Time        `json:"stopped_at,omitempty"`
	BytesRead    int64             `json:"bytes_read"`
	BytesWritten int64             `json:"bytes_written"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// Clone returns a deep copy of the AgentProcess metadata.
func (p *AgentProcess) Clone() *AgentProcess {
	if p == nil {
		return nil
	}
	cp := *p
	if p.Args != nil {
		cp.Args = make([]string, len(p.Args))
		copy(cp.Args, p.Args)
	}
	if p.Labels != nil {
		cp.Labels = make(map[string]string, len(p.Labels))
		for k, v := range p.Labels {
			cp.Labels[k] = v
		}
	}
	if p.ExitCode != nil {
		exitCode := *p.ExitCode
		cp.ExitCode = &exitCode
	}
	if p.StoppedAt != nil {
		stoppedAt := *p.StoppedAt
		cp.StoppedAt = &stoppedAt
	}
	return &cp
}

// AgentStopRequest instructs the supervisor to terminate a running agent.
type AgentStopRequest struct {
	AgentID             string `json:"agent_id"`
	Signal              string `json:"signal,omitempty"` // "SIGINT", "SIGTERM", "SIGKILL"
	GraceTimeoutSeconds int    `json:"grace_timeout_seconds,omitempty"`
}

// AgentInputRequest delivers raw input into a running agent's stdin.
type AgentInputRequest struct {
	AgentID string `json:"agent_id"`
	Data    string `json:"data"`
}
