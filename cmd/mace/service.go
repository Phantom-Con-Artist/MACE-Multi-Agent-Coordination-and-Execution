package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The worker runs as a systemd user service: started at boot (with linger),
// restarted after a crash, and independent of any terminal.

// serviceName is the unit's name; MACE_SERVICE_NAME overrides it so a second,
// isolated worker can be installed beside the owner's.
func serviceName() string { return env("MACE_SERVICE_NAME", "mace") + ".service" }

func unitPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "systemd", "user", serviceName()), nil
}

// unit renders the service file. PATH is captured from the shell running
// setup, because a user service otherwise starts with a minimal PATH and
// would not find the agent CLIs.
func (c settings) unit(executable, path string) string {
	var b strings.Builder
	b.WriteString("[Unit]\nDescription=MACE worker (Multi Agent Coordination and Execution)\n")
	b.WriteString("After=network-online.target\nWants=network-online.target\n\n[Service]\nType=forking\n")
	fmt.Fprintf(&b, "PIDFile=%s\n", c.PIDFile)
	fmt.Fprintf(&b, "ExecStart=%s start --port %d --host %s\n", quoteUnit(executable), c.Port, c.Host)
	fmt.Fprintf(&b, "ExecStop=%s stop\n", quoteUnit(executable))
	b.WriteString("Restart=on-failure\nRestartSec=5\nTimeoutStartSec=40\n")
	fmt.Fprintf(&b, "Environment=%s\n", quoteUnit("PATH="+path))
	b.WriteString("Environment=MACE_SERVICE=1\n")
	for _, key := range []string{"MACE_HOME", "MACE_PID_FILE", "MACE_LOG_FILE", "MACE_DB_PATH", "MACE_WEB_DIR", "MACE_DAEMON"} {
		if v := os.Getenv(key); v != "" {
			fmt.Fprintf(&b, "Environment=%s\n", quoteUnit(key+"="+v))
		}
	}
	b.WriteString("\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

// quoteUnit quotes a value for a systemd unit line when it needs it.
func quoteUnit(s string) string {
	if !strings.ContainsAny(s, " \t\"\\'") {
		return s
	}
	return strconv.Quote(s)
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// setup installs, enables, and starts the worker service.
func (c settings) setup(args []string) error {
	remove, linger := false, true
	for _, a := range args {
		switch a {
		case "--remove":
			remove = true
		case "--no-linger":
			linger = false
		}
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("systemd is not available here; run mace start instead")
	}
	path, err := unitPath()
	if err != nil {
		return err
	}
	if remove {
		systemctl("disable", "--now", serviceName())
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := systemctl("daemon-reload"); err != nil {
			return err
		}
		fmt.Printf("Removed %s. MACE no longer starts on its own; mace start still works.\n", serviceName())
		return nil
	}
	if c.Web == "" {
		return errors.New("web interface missing; run make install first")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if executable, err = filepath.EvalSymlinks(executable); err != nil {
		return err
	}
	// A server started by hand would hold the port the service needs.
	if c.owns(c.pid()) {
		fmt.Println("Stopping the MACE server started by hand, so the service can take over.")
		if err := c.stop(false); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(c.unit(executable, os.Getenv("PATH"))), 0600); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", "--now", serviceName()); err != nil {
		return fmt.Errorf("%w\nSee journalctl --user -u %s and mace logs", err, serviceName())
	}
	fmt.Printf("MACE runs as %s on port %d. It restarts after a crash and survives closing this terminal.\n", serviceName(), c.Port)
	if linger {
		user := env("USER", "")
		if out, err := exec.Command("loginctl", "enable-linger", user).CombinedOutput(); err != nil {
			fmt.Printf("It starts when you log in. To start it at boot before login, run: loginctl enable-linger %s (%s)\n", user, strings.TrimSpace(string(out)))
		} else {
			fmt.Println("It also starts at boot, before you log in.")
		}
	}
	fmt.Println("mace info prints the sign-in links. mace setup --remove undoes this.")
	return nil
}
