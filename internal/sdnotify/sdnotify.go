// Package sdnotify tells systemd that the daemon is ready and still
// healthy. When the unit sets WatchdogSec, systemd restarts a daemon whose
// pings stop, which catches a process that is alive but stuck, something
// Restart=on-failure alone never sees.
package sdnotify

import (
	"context"
	"net"
	"os"
	"strconv"
	"time"
)

type Notifier struct {
	socket string
	every  time.Duration
}

// FromEnv reads systemd's notification settings and removes them from the
// environment, so agents and tools this daemon starts cannot speak for it.
func FromEnv() Notifier {
	n := Notifier{socket: os.Getenv("NOTIFY_SOCKET")}
	// WATCHDOG_PID names the process systemd started, which with
	// Type=forking is the launcher, not this daemon, so it is not checked.
	if usec, err := strconv.ParseInt(os.Getenv("WATCHDOG_USEC"), 10, 64); err == nil && usec > 0 {
		n.every = time.Duration(usec) * time.Microsecond / 2
	}
	for _, k := range []string{"NOTIFY_SOCKET", "WATCHDOG_USEC", "WATCHDOG_PID"} {
		os.Unsetenv(k)
	}
	return n
}

// Notify sends one state line, such as READY=1. Without systemd it does
// nothing.
func (n Notifier) Notify(state string) error {
	if n.socket == "" {
		return nil
	}
	name := n.socket
	if name[0] == '@' {
		name = "\x00" + name[1:]
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: name, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write([]byte(state))
	return err
}

// Watch pings the watchdog at half its timeout for as long as healthy
// answers without error. It returns at once when no watchdog is set.
func (n Notifier) Watch(ctx context.Context, healthy func(context.Context) error) {
	if n.every <= 0 || n.socket == "" {
		return
	}
	tick := time.NewTicker(n.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		check, cancel := context.WithTimeout(ctx, min(n.every, 5*time.Second))
		err := healthy(check)
		cancel()
		if err == nil {
			n.Notify("WATCHDOG=1")
		}
	}
}
