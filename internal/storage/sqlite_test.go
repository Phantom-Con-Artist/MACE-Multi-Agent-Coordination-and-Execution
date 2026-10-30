package storage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mace/pkg/protocol"
)

func TestStore_AgentLifecycle(t *testing.T) {
	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	agent := &protocol.AgentProcess{
		ID:         "agent-test-1",
		Name:       "Test Runner",
		PID:        12345,
		Status:     protocol.AgentStatusRunning,
		Command:    "bash",
		Args:       []string{"-c", "echo hello"},
		WorkingDir: "/tmp",
		StartedAt:  time.Now().UTC(),
		Labels: map[string]string{
			"env": "test",
		},
	}

	// 1. Save agent
	if err := store.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("SaveAgent failed: %v", err)
	}

	// 2. Retrieve agent
	retrieved, err := store.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected agent, got nil")
	}
	if retrieved.ID != agent.ID || retrieved.PID != agent.PID || retrieved.Status != protocol.AgentStatusRunning {
		t.Errorf("agent fields mismatch: got %+v", retrieved)
	}
	if retrieved.Labels["env"] != "test" {
		t.Errorf("expected label env=test, got %v", retrieved.Labels["env"])
	}

	// 3. Update agent status
	exitCode := 0
	stoppedAt := time.Now().UTC()
	if err := store.UpdateAgentStatus(ctx, agent.ID, protocol.AgentStatusStopped, &exitCode, "", &stoppedAt); err != nil {
		t.Fatalf("UpdateAgentStatus failed: %v", err)
	}

	updated, err := store.GetAgent(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if updated.Status != protocol.AgentStatusStopped {
		t.Errorf("expected status stopped, got %v", updated.Status)
	}
	if updated.ExitCode == nil || *updated.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", updated.ExitCode)
	}

	// 4. List agents
	agents, err := store.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}

func TestStore_EventsAndAudit(t *testing.T) {
	ctx := context.Background()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer store.Close()

	agent := &protocol.AgentProcess{
		ID:         "agent-test-2",
		Name:       "Event Runner",
		PID:        54321,
		Status:     protocol.AgentStatusRunning,
		Command:    "python3",
		WorkingDir: "/tmp",
		StartedAt:  time.Now().UTC(),
	}
	if err := store.SaveAgent(ctx, agent); err != nil {
		t.Fatalf("SaveAgent failed: %v", err)
	}

	// Append 5 events
	for i := uint64(1); i <= 5; i++ {
		event := &protocol.AgentEvent{
			EventID:   fmt.Sprintf("evt-%d", i),
			AgentID:   agent.ID,
			Type:      protocol.EventStdout,
			Data:      fmt.Sprintf("Line output %d\n", i),
			Timestamp: time.Now().UTC(),
			Sequence:  i,
			Metadata: map[string]string{
				"seq": fmt.Sprintf("%d", i),
			},
		}
		if err := store.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent failed: %v", err)
		}
	}

	// Query events after seq 2 with limit 2
	events, err := store.GetAgentEvents(ctx, agent.ID, 2, 2)
	if err != nil {
		t.Fatalf("GetAgentEvents failed: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Sequence != 3 || events[1].Sequence != 4 {
		t.Errorf("expected sequences 3 and 4, got %d and %d", events[0].Sequence, events[1].Sequence)
	}

	// Test audit logging
	err = store.AppendAudit(ctx, "controller-1", "SPAWN_AGENT", agent.ID, map[string]interface{}{
		"command": agent.Command,
	})
	if err != nil {
		t.Fatalf("AppendAudit failed: %v", err)
	}
}
