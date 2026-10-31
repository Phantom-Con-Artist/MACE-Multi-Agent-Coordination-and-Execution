//go:build linux

package agent

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// setDeathSignal kills a direct child when the daemon dies. Grandchildren are
// handled by ReapOrphans on the next start.
func setDeathSignal(attr *syscall.SysProcAttr) { attr.Pdeathsig = syscall.SIGKILL }

// ProcessTag identifies children of this daemon instance for one installation.
func ProcessTag(installation string, daemonPID int) string {
	return installation + "/" + strconv.Itoa(daemonPID)
}

// ReapOrphans kills processes left behind by an earlier daemon of the same
// installation. A process is only touched when its environment carries this
// installation's tag and the daemon that started it is no longer alive.
func ReapOrphans(installation string) (int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	prefix := []byte("MACE_PROCESS_TAG=" + installation + "/")
	self := os.Getpid()
	killed := 0
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == self {
			continue
		}
		environ, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "environ"))
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(environ, []byte{0}) {
			if !bytes.HasPrefix(kv, prefix) {
				continue
			}
			owner, err := strconv.Atoi(strings.TrimSpace(string(kv[len(prefix):])))
			if err != nil || owner == self || alive(owner) {
				break
			}
			if syscall.Kill(pid, syscall.SIGKILL) == nil {
				killed++
			}
			break
		}
	}
	return killed, nil
}

func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
