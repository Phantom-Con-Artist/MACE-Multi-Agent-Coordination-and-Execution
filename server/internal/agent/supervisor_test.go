package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"mace/internal/storage"
	"mace/pkg/protocol"
)

func TestSupervisor_EchoExecution(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	supervisor := NewSupervisor(store, 5)
	defer supervisor.Close()

	req := protocol.AgentLaunchRequest{
		Name:    "echo-test",
		Command: "bash",
		Args:    []string{"-c", "echo 'mace agent output test'"},
	}

	proc, err := supervisor.StartAgent(ctx, req)
	if err != nil {
		t.Fatalf("StartAgent failed: %v", err)
	}

	// Subscribe to event stream
	_, unsubscribe := supervisor.Subscribe(proc.ID)
	defer unsubscribe()

	// Wait for completion (poll up to 3 seconds)
	var finalProc *protocol.AgentProcess
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		p, err := supervisor.GetAgent(proc.ID)
		if err != nil {
			t.Fatalf("GetAgent failed: %v", err)
		}
		if p.Status.IsTerminal() {
			finalProc = p
			break
		}
	}

	if finalProc == nil || finalProc.Status != protocol.AgentStatusStopped {
		t.Fatalf("expected status stopped, got %+v", finalProc)
	}
	if finalProc.ExitCode == nil || *finalProc.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %v", finalProc.ExitCode)
	}

	// Verify events were stored
	events, err := store.GetAgentEvents(ctx, proc.ID, 0, 10)
	if err != nil {
		t.Fatalf("GetAgentEvents failed: %v", err)
	}

	var foundStdout bool
	for _, e := range events {
		if e.Type == protocol.EventStdout && strings.Contains(e.Data, "mace agent output test") {
			foundStdout = true
			break
		}
	}
	if !foundStdout {
		t.Errorf("expected to find stdout with test output in events, got %d events", len(events))
	}
}

func TestSupervisor_InteractiveInput(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	supervisor := NewSupervisor(store, 5)
	defer supervisor.Close()

	// Launch cat (echoes stdin back to stdout)
	req := protocol.AgentLaunchRequest{
		Name:        "cat-runner",
		Command:     "cat",
		Interactive: true,
	}

	proc, err := supervisor.StartAgent(ctx, req)
	if err != nil {
		t.Fatalf("StartAgent failed: %v", err)
	}

	eventCh, unsubscribe := supervisor.Subscribe(proc.ID)
	defer unsubscribe()

	// Send input
	inputMsg := "interactive payload message\n"
	if err := supervisor.SendInput(proc.ID, inputMsg); err != nil {
		t.Fatalf("SendInput failed: %v", err)
	}

	// Read event stream for echoed output
	var receivedEcho bool
	timeout := time.After(3 * time.Second)
	for !receivedEcho {
		select {
		case ev := <-eventCh:
			if ev.Type == protocol.EventStdout && strings.Contains(ev.Data, "interactive payload message") {
				receivedEcho = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for echoed interactive output")
		}
	}

	// Stop cat
	if err := supervisor.StopAgent(proc.ID, "SIGTERM", 2*time.Second); err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	p, err := supervisor.GetAgent(proc.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if !p.Status.IsTerminal() {
		t.Errorf("expected terminal status, got %v", p.Status)
	}
}

func TestSupervisor_StopAndKill(t *testing.T) {
	ctx := context.Background()
	supervisor := NewSupervisor(nil, 5)
	defer supervisor.Close()

	req := protocol.AgentLaunchRequest{
		Name:    "sleeper",
		Command: "sleep",
		Args:    []string{"30"},
	}

	proc, err := supervisor.StartAgent(ctx, req)
	if err != nil {
		t.Fatalf("StartAgent failed: %v", err)
	}

	start := time.Now()
	if err := supervisor.StopAgent(proc.ID, "SIGKILL", 1*time.Second); err != nil {
		t.Fatalf("StopAgent failed: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Errorf("stop took too long: %v", elapsed)
	}

	p, err := supervisor.GetAgent(proc.ID)
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if !p.Status.IsTerminal() {
		t.Errorf("expected terminal status, got %v", p.Status)
	}
}

func TestSupervisor_CapacityLimiter(t *testing.T) {
	ctx := context.Background()
	// Supervisor with capacity of 1
	supervisor := NewSupervisor(nil, 1)
	defer supervisor.Close()

	req := protocol.AgentLaunchRequest{
		Command: "sleep",
		Args:    []string{"10"},
	}

	proc1, err := supervisor.StartAgent(ctx, req)
	if err != nil {
		t.Fatalf("StartAgent 1 failed: %v", err)
	}
	defer supervisor.StopAgent(proc1.ID, "SIGKILL", 1*time.Second)

	// Second start should fail immediately due to capacity limit
	_, err = supervisor.StartAgent(ctx, req)
	if err == nil {
		t.Fatal("expected ErrPoolCapacityFull, got nil")
	}
	if err != ErrPoolCapacityFull {
		t.Fatalf("expected ErrPoolCapacityFull, got %v", err)
	}
}
