package sdnotify

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func listen(t *testing.T) (*net.UnixConn, string) {
	path := filepath.Join(t.TempDir(), "notify")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, path
}

func TestPingsStopWhenTheDaemonIsUnhealthy(t *testing.T) {
	conn, path := listen(t)
	t.Setenv("NOTIFY_SOCKET", path)
	t.Setenv("WATCHDOG_USEC", "40000") // pings every 20ms
	n := FromEnv()
	if n.Notify("READY=1") != nil {
		t.Fatal("ready should be sent")
	}
	var sick atomic.Bool
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go n.Watch(ctx, func(context.Context) error {
		if sick.Load() {
			return errors.New("stuck")
		}
		return nil
	})
	read := func(wait time.Duration) string {
		buf := make([]byte, 64)
		conn.SetReadDeadline(time.Now().Add(wait))
		k, err := conn.Read(buf)
		if err != nil {
			return ""
		}
		return string(buf[:k])
	}
	if got := read(time.Second); got != "READY=1" {
		t.Fatalf("got %q", got)
	}
	if got := read(time.Second); got != "WATCHDOG=1" {
		t.Fatalf("a healthy daemon should ping, got %q", got)
	}
	sick.Store(true)
	time.Sleep(60 * time.Millisecond)
	for read(10*time.Millisecond) != "" {
	}
	if got := read(150 * time.Millisecond); got != "" {
		t.Fatalf("an unhealthy daemon must stop pinging, got %q", got)
	}
}

func TestWithoutSystemdNothingHappens(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")
	t.Setenv("WATCHDOG_USEC", "")
	n := FromEnv()
	if n.Notify("READY=1") != nil {
		t.Fatal("no socket means nothing to send")
	}
	n.Watch(context.Background(), nil) // returns at once
}
