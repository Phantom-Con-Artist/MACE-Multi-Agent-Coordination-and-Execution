//go:build !linux

package agent

import (
	"strconv"
	"syscall"
)

func setDeathSignal(*syscall.SysProcAttr) {}

// ProcessTag identifies children of this daemon instance for one installation.
func ProcessTag(installation string, daemonPID int) string {
	return installation + "/" + strconv.Itoa(daemonPID)
}

// ReapOrphans is only implemented on Linux.
func ReapOrphans(string) (int, error) { return 0, nil }
