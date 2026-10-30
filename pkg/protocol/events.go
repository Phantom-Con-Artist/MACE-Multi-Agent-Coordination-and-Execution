package protocol

import "time"

// EventType specifies the nature of an agent event.
type EventType string

const (
	EventStdout EventType = "stdout"
	EventStderr EventType = "stderr"
	EventStatus EventType = "status"
	EventExit   EventType = "exit"
	EventError  EventType = "error"
)

// AgentEvent represents a single event emitted during agent execution.
type AgentEvent struct {
	EventID   string            `json:"event_id"`
	AgentID   string            `json:"agent_id"`
	Type      EventType         `json:"type"`
	Data      string            `json:"data"`
	Timestamp time.Time         `json:"timestamp"`
	Sequence  uint64            `json:"sequence"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}
