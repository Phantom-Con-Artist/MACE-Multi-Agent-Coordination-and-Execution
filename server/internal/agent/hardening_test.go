package agent

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"mace/internal/storage"
	"mace/pkg/protocol"
)

func waitTerminal(t *testing.T, s *Supervisor, id string, limit time.Duration) *protocol.AgentProcess {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		p, err := s.GetAgent(id)
		if err == nil && p.Status.IsTerminal() {
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("process %s did not finish within %s", id, limit)
	return nil
}

func storedOutput(t *testing.T, store *storage.Store, id string) (string, int) {
	t.Helper()
	events, err := store.GetAgentEvents(context.Background(), id, 0, 100000)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	partial := 0
	for _, e := range events {
		if e.Type != protocol.EventStdout {
			continue
		}
		out.WriteString(e.Data)
		if e.Metadata["partial"] == "true" {
			partial++
		} else {
			out.WriteByte('\n')
		}
	}
	return out.String(), partial
}

// A single line far over the old 1 MiB scanner limit must not stall the child.
func TestSupervisor_OversizedLineIsDrained(t *testing.T) {
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := NewSupervisor(store, 2)
	defer s.Close()
	size := 3<<20 + 123
	proc, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{
		Command: "sh", Args: []string{"-c", "head -c " + strconv.Itoa(size) + " /dev/zero | tr '\\0' x; echo; echo after"},
		TimeoutSeconds: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := waitTerminal(t, s, proc.ID, 15*time.Second)
	if p.ExitCode == nil || *p.ExitCode != 0 {
		t.Fatalf("expected clean exit, got %+v", p)
	}
	out, partial := storedOutput(t, store, proc.ID)
	if partial == 0 {
		t.Fatal("expected the long line to be split into partial chunks")
	}
	if out != strings.Repeat("x", size)+"\nafter\n" {
		t.Fatalf("reassembled output mismatch: got %d bytes", len(out))
	}
}

// Children must never see Mace credentials, and must carry the process tag.
func TestSupervisor_ChildEnvironmentHasNoSecrets(t *testing.T) {
	t.Setenv("MACE_AUTH_SECRET", "top-secret-token")
	t.Setenv("MACE_TS_AUTHKEY", "tskey-secret")
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s := NewSupervisor(store, 2)
	s.SetProcessTag("test-install/1")
	defer s.Close()
	proc, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{
		Command: "env", Env: map[string]string{"MACE_INJECTED": "x", "VISIBLE": "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, s, proc.ID, 5*time.Second)
	out, _ := storedOutput(t, store, proc.ID)
	for _, secret := range []string{"top-secret-token", "tskey-secret", "MACE_AUTH_SECRET", "MACE_INJECTED"} {
		if strings.Contains(out, secret) {
			t.Fatalf("child environment leaked %q", secret)
		}
	}
	if !strings.Contains(out, "VISIBLE=yes") || !strings.Contains(out, "MACE_PROCESS_TAG=test-install/1") {
		t.Fatalf("expected requested env and process tag, got:\n%s", out)
	}
}

// A caller holding a reserved slot launches once capacity frees, instead of failing.
func TestSupervisor_ReservedSlotWaitsForCapacity(t *testing.T) {
	s := NewSupervisor(nil, 1)
	defer s.Close()
	first, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"0.3"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := time.Now()
	if err := s.AcquireSlot(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) < 200*time.Millisecond {
		t.Fatal("slot should only be granted after the first process exits")
	}
	second, err := s.StartAgent(ctx, protocol.AgentLaunchRequest{Command: "true", Reserved: true})
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, s, first.ID, 5*time.Second)
	waitTerminal(t, s, second.ID, 5*time.Second)
	// The slot is released on exit, so a third launch succeeds immediately.
	third, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "true"})
	if err != nil {
		t.Fatalf("slot leaked: %v", err)
	}
	waitTerminal(t, s, third.ID, 5*time.Second)

	// A cancelled wait returns without holding a slot.
	blocker, _ := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"5"}})
	short, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if err := s.AcquireSlot(short); err == nil {
		t.Fatal("expected cancelled wait")
	}
	s.StopAgent(blocker.ID, "SIGKILL", time.Second)
}

func TestSupervisor_IdleTimeoutStopsSilentProcess(t *testing.T) {
	s := NewSupervisor(nil, 1)
	defer s.Close()
	proc, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"30"}, IdleTimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := waitTerminal(t, s, proc.ID, 10*time.Second)
	if !strings.Contains(p.Error, "no output") {
		t.Fatalf("expected idle reason, got %q", p.Error)
	}
}

func TestSupervisor_TimeLimitIsReported(t *testing.T) {
	s := NewSupervisor(nil, 1)
	defer s.Close()
	proc, err := s.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "sleep", Args: []string{"30"}, TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	p := waitTerminal(t, s, proc.ID, 10*time.Second)
	if p.Error != "time limit reached" {
		t.Fatalf("expected time limit reason, got %q", p.Error)
	}
}

func TestReapOrphans_OnlyKillsTaggedOrphans(t *testing.T) {
	if _, err := os.Stat("/proc/self/environ"); err != nil {
		t.Skip("requires /proc")
	}
	install := "reap-test-" + strconv.Itoa(os.Getpid())
	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	start := func(tag string) *exec.Cmd {
		cmd := exec.Command("sleep", "30")
		cmd.Env = append(os.Environ(), "MACE_PROCESS_TAG="+tag)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		return cmd
	}
	orphan := start(ProcessTag(install, dead.Process.Pid))
	living := start(ProcessTag(install, os.Getppid()))
	other := start(ProcessTag("another-install", dead.Process.Pid))
	n, err := ReapOrphans(install)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected exactly one orphan killed, got %d", n)
	}
	if err := orphan.Wait(); err == nil {
		t.Fatal("orphan should have been killed")
	}
	for _, cmd := range []*exec.Cmd{living, other} {
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			t.Fatalf("process %d should still be running", cmd.Process.Pid)
		}
	}
}
