//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

// prepareCmdPlatform sets process group attributes on Unix systems so all child processes
// can be signaled together.
func prepareCmdPlatform(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessPlatform terminates the entire process group cleanly.
func killProcessPlatform(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	// Signal the entire process group
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		return syscall.Kill(-pgid, sig)
	}
	return cmd.Process.Signal(sig)
}
