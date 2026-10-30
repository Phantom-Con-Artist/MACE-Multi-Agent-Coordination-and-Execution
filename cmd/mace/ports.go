package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type daemonPort struct {
	PID        int
	Port       int
	Executable string
}

// Resolve listening sockets back to maced file descriptors. A port number alone
// never authorizes terminating an arbitrary process or another kind of server.
func macePorts() []daemonPort {
	sockets := map[string]int{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, _ := os.ReadFile(path)
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != "0A" {
				continue
			}
			_, hexPort, ok := strings.Cut(f[1], ":")
			if !ok {
				continue
			}
			port, e := strconv.ParseInt(hexPort, 16, 32)
			if e == nil {
				sockets[f[9]] = int(port)
			}
		}
	}
	out := []daemonPort{}
	entries, _ := os.ReadDir("/proc")
	for _, entry := range entries {
		pid, e := strconv.Atoi(entry.Name())
		if e != nil || !alive(pid) {
			continue
		}
		exe, e := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if e != nil {
			continue
		}
		exe = strings.TrimSuffix(exe, " (deleted)")
		if filepath.Base(exe) != "maced" {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		seen := map[int]bool{}
		for _, fd := range fds {
			link, e := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fd.Name()))
			if e != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			if port, ok := sockets[inode]; ok && !seen[port] {
				out = append(out, daemonPort{PID: pid, Port: port, Executable: exe})
				seen[port] = true
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}
func listPorts() error {
	rows := macePorts()
	if len(rows) == 0 {
		fmt.Println("No local Mace TCP listeners. No ports need to be cleared.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tPID\tSERVER")
	for _, row := range rows {
		fmt.Fprintf(w, "%d\t%d\t%s\n", row.Port, row.PID, row.Executable)
	}
	w.Flush()
	fmt.Println("Stop one: mace stop --port <port>. Force it only if needed: mace kill --port <port>.")
	return nil
}
func portFlag(args []string) bool {
	for _, arg := range args {
		key, _, _ := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if key == "port" {
			return true
		}
	}
	return false
}
func (c settings) stopPort(force bool) error {
	if c.Port < 1 {
		return fmt.Errorf("choose a specific port between 1 and 65535")
	}
	for _, row := range macePorts() {
		if row.Port != c.Port {
			continue
		}
		current, e := os.Readlink(fmt.Sprintf("/proc/%d/exe", row.PID))
		if e != nil || strings.TrimSuffix(current, " (deleted)") != row.Executable {
			return fmt.Errorf("process changed while checking port %d; run mace ports again", c.Port)
		}
		signal := syscall.SIGTERM
		if force {
			killDescendants(row.PID)
			signal = syscall.SIGKILL
		}
		if e := syscall.Kill(row.PID, signal); e != nil {
			return e
		}
		for i := 0; i < 100 && alive(row.PID); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if alive(row.PID) {
			return fmt.Errorf("Mace on port %d is still stopping; use mace kill --port %d if needed", c.Port, c.Port)
		}
		if c.pid() == row.PID {
			os.Remove(c.PIDFile)
		}
		fmt.Printf("Stopped Mace on port %d (PID %d). The listener is closed.\n", c.Port, row.PID)
		return nil
	}
	ln, e := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(c.Port)))
	if e == nil {
		ln.Close()
		fmt.Printf("No Mace server is listening on port %d. It is already free.\n", c.Port)
		return nil
	}
	return fmt.Errorf("port %d is occupied, but no accessible local maced process owns it; no unrelated process was stopped", c.Port)
}
func (c settings) statusPort() error {
	for _, row := range macePorts() {
		if row.Port != c.Port {
			continue
		}
		fmt.Printf("Mace listener: port %d | PID %d\nExecutable: %s\n", row.Port, row.PID, row.Executable)
		h, e := health("http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(row.Port)))
		if e != nil {
			return fmt.Errorf("process is listening but health check failed: %w", e)
		}
		fmt.Printf("Health: ready | MACE %v | node %v\n", h["version"], h["node_id"])
		return nil
	}
	return fmt.Errorf("no accessible local Mace server listens on port %d; run mace ports", c.Port)
}
