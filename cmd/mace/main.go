// mace is the user-facing host lifecycle and inspection CLI.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mace/internal/bootstrap"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

type settings struct {
	Host    string
	Port    int
	DB      string
	Daemon  string
	Web     string
	Log     string
	PIDFile string
	State   string
	Tailnet bool
	Secret  string
	Args    []string
}
type runtimeInfo struct {
	PID     int       `json:"pid"`
	URL     string    `json:"url"`
	Host    string    `json:"host"`
	Port    int       `json:"port"`
	Daemon  string    `json:"daemon"`
	Log     string    `json:"log"`
	DB      string    `json:"database"`
	Started time.Time `json:"started"`
	Args    []string  `json:"args"`
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	run := run
	if filepath.Base(os.Args[0]) == "mace-server" {
		run = serverCommand
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mace:", err)
		os.Exit(1)
	}
}
func loadSettings(args []string) (settings, error) {
	executable, e := os.Executable()
	if e != nil {
		return settings{}, e
	}
	c := settings{Host: env("MACE_HTTP_HOST", "0.0.0.0"), DB: env("MACE_DB_PATH", filepath.Join(bootstrap.DataDir(), "mace.db")), Daemon: env("MACE_DAEMON", filepath.Join(filepath.Dir(executable), "maced")), Web: os.Getenv("MACE_WEB_DIR"), Log: env("MACE_LOG_FILE", filepath.Join(bootstrap.DataDir(), "maced.log")), PIDFile: env("MACE_PID_FILE", filepath.Join(bootstrap.DataDir(), "maced.pid")), Tailnet: os.Getenv("MACE_TS_AUTHKEY") != "", Secret: os.Getenv("MACE_AUTH_SECRET"), Args: append([]string{}, args...)}
	port, e := strconv.Atoi(env("MACE_HTTP_PORT", "8080"))
	if e != nil {
		return c, fmt.Errorf("MACE_HTTP_PORT must be a number")
	}
	c.Port = port
	c.State = filepath.Dir(c.PIDFile)
	for i := 0; i < len(args); i++ {
		key, value, has := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		if key != "host" && key != "port" && key != "db" && key != "auth-secret" && key != "ts-authkey" {
			continue
		}
		if !has {
			if i+1 >= len(args) {
				return c, fmt.Errorf("--%s needs a value", key)
			}
			i++
			value = args[i]
		}
		switch key {
		case "host":
			c.Host = value
		case "port":
			c.Port, e = strconv.Atoi(value)
			if e != nil {
				return c, fmt.Errorf("port must be a number")
			}
		case "db":
			c.DB = value
		case "auth-secret":
			c.Secret = value
		case "ts-authkey":
			c.Tailnet = value != ""
		}
	}
	if c.Port < 0 || c.Port > 65535 {
		return c, fmt.Errorf("port must be between 0 and 65535")
	}
	c.Daemon, e = filepath.Abs(c.Daemon)
	if e != nil {
		return c, e
	}
	c.DB, e = filepath.Abs(c.DB)
	if e != nil {
		return c, e
	}
	if c.Web == "" {
		for _, p := range []string{filepath.Join(filepath.Dir(executable), "../web/dist"), filepath.Join(filepath.Dir(executable), "../share/mace/web"), "web/dist"} {
			if _, e := os.Stat(filepath.Join(p, "index.html")); e == nil {
				c.Web, _ = filepath.Abs(p)
				break
			}
		}
	}
	return c, nil
}
func (c settings) info() runtimeInfo {
	var r runtimeInfo
	b, _ := os.ReadFile(filepath.Join(c.State, "runtime.json"))
	json.Unmarshal(b, &r)
	return r
}
func (c settings) pid() int {
	b, _ := os.ReadFile(c.PIDFile)
	p, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return p
}
func alive(pid int) bool {
	if pid <= 1 {
		return false
	}
	if b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); e == nil {
		end := strings.LastIndex(string(b), ")")
		if end >= 0 && len(b) > end+2 && b[end+2] == 'Z' {
			return false
		}
	}
	e := syscall.Kill(pid, 0)
	return e == nil || e == syscall.EPERM
}
func (c settings) owns(pid int) bool {
	if !alive(pid) {
		return false
	}
	target, e := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if e != nil {
		return false
	}
	expected := c.Daemon
	if r := c.info(); r.PID == pid && r.Daemon != "" {
		expected = r.Daemon
	}
	expected, _ = filepath.EvalSymlinks(expected)
	return strings.TrimSuffix(target, " (deleted)") == expected
}
func (c settings) localURL() string {
	host := c.Host
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(c.Port))
}
func (c settings) token() string {
	if b, e := os.ReadFile(filepath.Join(c.State, "active-token")); e == nil {
		return string(b)
	}
	if c.Secret != "" {
		return c.Secret
	}
	b, _ := os.ReadFile(filepath.Join(c.State, "access-token"))
	return string(b)
}
func health(address string) (map[string]any, error) {
	client := http.Client{Timeout: 800 * time.Millisecond}
	r, e := client.Get(address + "/health")
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", r.StatusCode)
	}
	var out map[string]any
	e = json.NewDecoder(io.LimitReader(r.Body, 65536)).Decode(&out)
	if e != nil {
		return nil, e
	}
	if out["application"] != "mace" || out["status"] != "ok" {
		return nil, errors.New("listener is not a Mace server")
	}
	return out, nil
}
func save(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(path, b, 0600)
}
func (c settings) start() error {
	if p := c.pid(); c.owns(p) {
		r := c.info()
		fmt.Printf("MACE is already running (PID %d).\nAddress: %s\nUse mace info, mace logs, or mace restart.\n", p, r.URL)
		return nil
	}
	if e := os.MkdirAll(c.State, 0700); e != nil {
		return e
	}
	lock, e := os.OpenFile(c.PIDFile+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("another mace lifecycle command is in progress; run mace status")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if p := c.pid(); c.owns(p) {
		return fmt.Errorf("MACE is already running (PID %d)", p)
	}
	if _, e := os.Stat(c.Daemon); e != nil {
		return fmt.Errorf("daemon executable unavailable at %s; reinstall Mace", c.Daemon)
	}
	if c.Web == "" {
		return errors.New("web interface missing; run make build or reinstall Mace")
	}
	if !c.Tailnet {
		ln, e := net.Listen("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
		if e != nil {
			if h, probe := health(c.localURL()); probe == nil {
				return fmt.Errorf("port %d already serves Mace (%v); use its address or choose a free port with mace start --port 0", c.Port, h["node_id"])
			}
			return fmt.Errorf("cannot bind %s:%d: %w; use mace doctor --port %d or choose another --port", c.Host, c.Port, e, c.Port)
		}
		if c.Port == 0 {
			c.Port = ln.Addr().(*net.TCPAddr).Port
			c.Args = append(c.Args, "--port", strconv.Itoa(c.Port))
		}
		ln.Close()
	}
	token := c.Secret
	if token == "" {
		token, e = bootstrap.Token(filepath.Join(c.State, "access-token"))
		if e != nil {
			return e
		}
	}
	if e := os.MkdirAll(filepath.Dir(c.Log), 0700); e != nil {
		return e
	}
	log, e := os.OpenFile(c.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer log.Close()
	ready := filepath.Join(c.State, "ready.json")
	os.Remove(ready)
	// The token reaches the daemon through a private file, never the environment:
	// /proc/<pid>/environ stays readable to every process of the same user.
	secretFile := filepath.Join(c.State, "active-token")
	if e := os.MkdirAll(c.State, 0700); e != nil {
		return e
	}
	if e := os.WriteFile(secretFile, []byte(token), 0600); e != nil {
		return e
	}
	secretEnv := "MACE_AUTH_SECRET_FILE=" + secretFile
	cmd := exec.Command(c.Daemon, c.Args...)
	cmd.Env = append(withoutEnv(os.Environ(), "MACE_AUTH_SECRET"), secretEnv, "MACE_WEB_DIR="+c.Web, "MACE_READY_FILE="+ready)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = log
	cmd.Stderr = log
	if e := cmd.Start(); e != nil {
		return e
	}
	pid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	cleanup := func() { cmd.Process.Kill(); os.Remove(c.PIDFile); os.Remove(ready) }
	if e := os.WriteFile(c.PIDFile, []byte(strconv.Itoa(pid)), 0600); e != nil {
		cleanup()
		return e
	}
	deadline := time.Now().Add(20 * time.Second)
	address := c.localURL()
	if c.Tailnet {
		address = "http://" + net.JoinHostPort(env("MACE_TS_HOSTNAME", "mace-node"), strconv.Itoa(c.Port))
	}
	for {
		select {
		case e := <-done:
			os.Remove(c.PIDFile)
			return fmt.Errorf("daemon exited before becoming ready (%v); run mace logs", e)
		default:
		}
		isReady := false
		if c.Tailnet {
			_, e := os.Stat(ready)
			isReady = e == nil
		} else {
			h, e := health(address)
			isReady = e == nil && strings.HasSuffix(fmt.Sprint(h["node_id"]), "-"+strconv.Itoa(pid))
		}
		if isReady {
			break
		}
		if time.Now().After(deadline) {
			cleanup()
			return fmt.Errorf("server did not become ready in 20 seconds; run mace logs or mace doctor")
		}
		time.Sleep(100 * time.Millisecond)
	}
	record := runtimeInfo{PID: pid, URL: address, Host: c.Host, Port: c.Port, Daemon: c.Daemon, Log: c.Log, DB: c.DB, Started: time.Now().UTC(), Args: c.Args}
	if e := save(filepath.Join(c.State, "runtime.json"), record); e != nil {
		cleanup()
		return e
	}
	if e := os.WriteFile(filepath.Join(c.State, "active-token"), []byte(token), 0600); e != nil {
		cleanup()
		return e
	}
	// Under the service this output goes to the journal, which administrators
	// can read, so the token and links stay out of it; mace info prints them.
	// The unit sets MACE_SERVICE; INVOCATION_ID is not used because desktop
	// sessions pass it on to every terminal.
	if os.Getenv("MACE_SERVICE") == "" {
		bootstrap.Banner(token, c.Host, c.Port, c.Log)
	}
	fmt.Printf("Server ready (PID %d). State: %s\n", pid, c.State)
	return nil
}
func (c settings) stop(force bool) error {
	pid := c.pid()
	if !c.owns(pid) {
		if _, e := os.Stat(c.PIDFile); e == nil {
			os.Remove(c.PIDFile)
			fmt.Println("Cleared a stale Mace PID record. No unrelated process was signalled.")
		}
		fmt.Println("MACE is already stopped.")
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		killDescendants(pid)
		sig = syscall.SIGKILL
	}
	if e := syscall.Kill(pid, sig); e != nil {
		return e
	}
	for i := 0; i < 100 && alive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if alive(pid) {
		return errors.New("server is still stopping; use mace kill to force it")
	}
	if e := os.Remove(c.PIDFile); e != nil && !os.IsNotExist(e) {
		return e
	}
	fmt.Println("MACE stopped")
	return nil
}
func killDescendants(pid int) {
	tasks, _ := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	children := map[string]bool{}
	for _, task := range tasks {
		b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/task/%s/children", pid, task.Name()))
		for _, child := range strings.Fields(string(b)) {
			children[child] = true
		}
	}
	for s := range children {
		child, _ := strconv.Atoi(s)
		if child <= 1 {
			continue
		}
		killDescendants(child)
		if group, e := syscall.Getpgid(child); e == nil && group == child {
			syscall.Kill(-group, syscall.SIGKILL)
		} else {
			syscall.Kill(child, syscall.SIGKILL)
		}
	}
}
func (c settings) request(path string, out any) error {
	return c.call("GET", path, 3*time.Second, out)
}
func (c settings) call(method, path string, timeout time.Duration, out any) error {
	return c.send(method, path, timeout, nil, out)
}

// send is call with a JSON body.
func (c settings) send(method, path string, timeout time.Duration, body, out any) error {
	r := c.info()
	if !c.owns(c.pid()) {
		return errors.New("server is stopped; run mace start")
	}
	url := r.URL
	if url == "" {
		url = c.localURL()
	}
	var data io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		data = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, url+"/api/v1"+path, data)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token())
	client := http.Client{Timeout: timeout}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 1000))
		if res.StatusCode >= 400 && res.StatusCode < 500 {
			return errors.New(strings.TrimSpace(string(msg)))
		}
		return fmt.Errorf("host returned %d: %s", res.StatusCode, msg)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
func (c settings) status() error {
	pid := c.pid()
	if !c.owns(pid) {
		// stop removes the PID record, so one left behind means the server
		// ended without a clean shutdown.
		if r := c.info(); pid > 0 && r.PID == pid {
			return fmt.Errorf("MACE stopped unexpectedly: PID %d, started %s, exited without a clean shutdown. See mace logs, then run mace start",
				pid, r.Started.Local().Format(time.RFC1123))
		}
		return errors.New("MACE is stopped; run mace start")
	}
	r := c.info()
	fmt.Printf("MACE %s | Multi Agent Coordination and Execution\nPID: %d\nAddress: %s\nStarted: %s\nLog: %s\nDatabase: %s\n", bootstrap.Version, pid, r.URL, r.Started.Local().Format(time.RFC1123), r.Log, r.DB)
	h, e := health(r.URL)
	if e != nil {
		return fmt.Errorf("process is alive but HTTP health check failed: %w; run mace logs", e)
	}
	fmt.Printf("Health: ready | node %v | uptime %vs\n", h["node_id"], h["uptime_seconds"])
	return nil
}
func (c settings) logs(args []string) error {
	count := 50
	follow := false
	for i := 0; i < len(args); i++ {
		key, value, has := strings.Cut(strings.TrimLeft(args[i], "-"), "=")
		switch key {
		case "follow", "f":
			follow = true
		case "lines", "n":
			if !has {
				if i+1 >= len(args) {
					return errors.New("--lines needs a number")
				}
				i++
				value = args[i]
			}
			n, e := strconv.Atoi(value)
			if e != nil || n < 1 || n > 10000 {
				return errors.New("--lines must be between 1 and 10000")
			}
			count = n
		default:
			return fmt.Errorf("unknown logs option %q", args[i])
		}
	}
	path := c.Log
	if r := c.info(); r.Log != "" {
		path = r.Log
	}
	f, e := os.Open(path)
	if e != nil {
		return fmt.Errorf("log unavailable at %s: %w", path, e)
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return e
	}
	start := info.Size() - 4*1024*1024
	if start < 0 {
		start = 0
	}
	f.Seek(start, 0)
	data, e := io.ReadAll(f)
	if e != nil {
		return e
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	fmt.Println(strings.Join(lines, "\n"))
	if !follow {
		return nil
	}
	fmt.Println("Following log. Press Ctrl+C to stop.")
	for {
		if _, e := io.Copy(os.Stdout, f); e != nil {
			return e
		}
		time.Sleep(300 * time.Millisecond)
	}
}
func (c settings) doctor() error {
	bootstrap.Brand()
	fmt.Printf("Daemon: %s\n", c.Daemon)
	if _, e := os.Stat(c.Daemon); e != nil {
		fmt.Println("  Missing. Reinstall Mace.")
	}
	fmt.Printf("Web interface: %s\nState: %s\n", c.Web, c.State)
	p := c.pid()
	if c.owns(p) {
		fmt.Printf("Managed process: running (PID %d)\n", p)
	} else if p != 0 {
		fmt.Printf("Managed process: stale PID %d, safe to run mace start\n", p)
	} else {
		fmt.Println("Managed process: stopped")
	}
	if !c.Tailnet {
		ln, e := net.Listen("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)))
		if e == nil {
			fmt.Printf("Port %d: available\n", c.Port)
			ln.Close()
		} else {
			fmt.Printf("Port %d: occupied or unavailable (%v)\n", c.Port, e)
			if h, e := health(c.localURL()); e == nil {
				fmt.Printf("Listener: Mace %v (%v)\n", h["version"], h["node_id"])
			}
			fmt.Println("Choose a free port with mace start --port 0.")
		}
	}
	fmt.Println("Installed agents:")
	for _, name := range []string{"codex", "claude", "agy", "muse"} {
		path, e := exec.LookPath(name)
		if e != nil {
			path = "not found on PATH"
		}
		fmt.Printf("  %-8s %s\n", name, path)
	}
	return nil
}
func help() {
	bootstrap.Brand()
	fmt.Print("Usage: mace <command> [options]\n\n  start [--port 8080] [--host 0.0.0.0]  Start and print sign-in links\n  setup [--port 8080] [--remove]        Run MACE as a background service\n  join --invite                         Print a code for adding another machine\n  join <address> <code>                 Add this machine to your other machines\n  machines                              List joined machines and who is available\n  status [--port 8080]                  Check PID, address, and HTTP health\n  ports                                 List local Mace listeners\n  info                                  Print current links and token\n  token                                 Print the current access token\n  agents                                List conversational agents\n  ps                                    List native processes\n  logs [--lines 50] [--follow]            Inspect host logs\n  doctor [--port 8080]                   Check port, paths, and CLI discovery\n  restart [daemon options]              Restart with the saved settings\n  stop [--port 8080]                    Stop one Mace server gracefully\n  pause-all                             Stop all agent work here and on connected hosts\n  kill [--port 8080]                    Force stop Mace and its descendants\n  server start|qr|status|stop           Run this machine as the hub your phone opens (also mace-server)\n  ctx <view> [words]                    Fetch conversation context during an agent turn\n  version                               Print name, abbreviation, and version\n  maced [options]                       Run the foreground daemon\n  help                                  Show these commands\n\nStart examples: mace start | mace start --port 8081 | mace start --port 0\nFor all daemon options: mace maced --help\n")
}
func run(args []string) error {
	command := "help"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	if command == "help" || command == "--help" || command == "-h" {
		help()
		return nil
	}
	if command == "version" || command == "--version" || command == "-v" {
		bootstrap.Brand()
		return nil
	}
	if command == "start" {
		for _, a := range args {
			if a == "--help" || a == "-h" {
				help()
				return nil
			}
		}
	}
	if command == "ctx" {
		return contextFetch(args)
	}
	if command == "server" {
		return serverCommand(args)
	}
	c, e := loadSettings(args)
	if e != nil {
		return e
	}
	switch command {
	case "start":
		return c.start()
	case "setup":
		return c.setup(args)
	case "join":
		return c.join(args)
	case "nodes", "machines":
		return c.nodes()
	case "stop":
		if portFlag(args) {
			return c.stopPort(false)
		}
		return c.stop(false)
	case "kill":
		if portFlag(args) {
			return c.stopPort(true)
		}
		return c.stop(true)
	case "restart":
		if len(args) == 0 {
			c, e = loadSettings(c.info().Args)
			if e != nil {
				return e
			}
		}
		if e := c.stop(false); e != nil {
			return e
		}
		return c.start()
	case "status":
		if portFlag(args) {
			return c.statusPort()
		}
		return c.status()
	case "ports":
		return listPorts()
	case "doctor":
		return c.doctor()
	case "info":
		if !c.owns(c.pid()) {
			return errors.New("server is stopped; run mace start")
		}
		r := c.info()
		bootstrap.Banner(c.token(), r.Host, r.Port, r.Log)
		return nil
	case "token":
		if c.token() == "" {
			return errors.New("no saved token; run mace start")
		}
		fmt.Println(c.token())
		return nil
	case "logs":
		return c.logs(args)
	case "agents", "ps":
		path := "/sessions"
		if command == "ps" {
			path = "/agents/"
		}
		var rows []map[string]any
		if e := c.request(path, &rows); e != nil {
			return e
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tFOLDER")
		for _, a := range rows {
			if a["status"] == "removed" {
				continue
			}
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", a["id"], a["name"], a["status"], a["working_dir"])
		}
		w.Flush()
		if len(rows) == 0 {
			fmt.Println("No agents yet. Open Chats and choose +.")
		}
		return nil
	case "pause-all":
		var out struct {
			Runs      int               `json:"runs"`
			Processes int               `json:"processes"`
			Peers     map[string]string `json:"peers"`
		}
		if e := c.call("POST", "/pause-all", 30*time.Second, &out); e != nil {
			return e
		}
		fmt.Printf("Paused %d conversations and stopped %d agent processes on this host.\n", out.Runs, out.Processes)
		for peer, result := range out.Peers {
			fmt.Printf("  %s: %s\n", peer, result)
		}
		fmt.Println("Open a conversation and choose Continue to resume.")
		return nil
	case "maced":
		cmd := exec.Command(c.Daemon, args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	default:
		return fmt.Errorf("unknown command %q; run mace help", command)
	}
}

func withoutEnv(env []string, keys ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		keep := true
		for _, k := range keys {
			if strings.HasPrefix(kv, k+"=") {
				keep = false
			}
		}
		if keep {
			out = append(out, kv)
		}
	}
	return out
}
