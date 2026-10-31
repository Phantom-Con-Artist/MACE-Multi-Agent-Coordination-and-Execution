package protocol

import "time"

// NodeRole represents the operational role of a MACE node.
type NodeRole string

const (
	NodeRoleController NodeRole = "controller"
	NodeRoleWorker     NodeRole = "worker"
)

// NodeInfo contains system and capability metadata about a MACE node.
type NodeInfo struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Role             NodeRole  `json:"role"`
	HostName         string    `json:"hostname"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	Version          string    `json:"version"`
	ActiveAgents     int       `json:"active_agents"`
	TotalMemoryBytes uint64    `json:"total_memory_bytes"`
	StartedAt        time.Time `json:"started_at"`
	UptimeSeconds    uint64    `json:"uptime_seconds"`
}
