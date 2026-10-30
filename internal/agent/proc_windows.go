//go:build windows

package agent

import (
	"os/exec"
	"syscall"
)

func prepareCmdPlatform(cmd *exec.Cmd) {
	// Windows default
}

func killProcessPlatform(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
