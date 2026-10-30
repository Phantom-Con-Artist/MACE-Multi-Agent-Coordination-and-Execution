package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mace/pkg/protocol"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Session struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Directory string `json:"working_dir"`
	Status    string `json:"status"`
}
type Provider struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
}

func providers() []Provider {
	out := []Provider{}
	for _, p := range []Provider{{ID: "codex", Name: "Codex"}, {ID: "claude", Name: "Claude Code"}, {ID: "agy", Name: "Antigravity"}, {ID: "muse", Name: "Muse"}} {
		_, err := exec.LookPath(p.ID)
		p.Available = err == nil
		out = append(out, p)
	}
	if os.Getenv("MACE_TEST_MODE") == "1" {
		out = append(out, Provider{ID: "diagnostic", Name: "Diagnostic echo", Available: true})
	}
	return out
}
func (h *APIHandler) catalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"providers": providers(), "directory": "/"})
}
func (h *APIHandler) sessions(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := h.store.Documents(r.Context(), "session")
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		writeJSON(w, 200, v)
		return
	}
	var s Session
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&s) != nil {
		http.Error(w, "invalid session", 400)
		return
	}
	valid := false
	for _, p := range providers() {
		if p.ID == s.Provider && p.Available {
			valid = true
			if s.Name == "" {
				s.Name = p.Name
			}
		}
	}
	if !valid {
		http.Error(w, "agent is not installed on this host", 400)
		return
	}
	previous, _ := h.store.Documents(r.Context(), "session")
	count := 0
	for _, b := range previous {
		var other Session
		if json.Unmarshal(b, &other) == nil && other.Provider == s.Provider && other.Status != "removed" {
			count++
		}
	}
	if count > 0 {
		s.Name = fmt.Sprintf("%s %d", s.Name, count+1)
	}
	if s.Directory == "" {
		s.Directory = "/"
	}
	absolute, e := filepath.Abs(s.Directory)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	info, e := os.Stat(absolute)
	if e != nil || !info.IsDir() {
		http.Error(w, "folder unavailable", 400)
		return
	}
	s.Directory = absolute
	s.ID = "session_" + newID()
	s.Status = "ready"
	if e := h.store.PutDocument(r.Context(), "session", s.ID, s); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "SESSION_CREATED", s.ID, map[string]any{"provider": s.Provider, "directory": s.Directory})
	writeJSON(w, 201, s)
}
func (h *APIHandler) audit(actor, action, target string, details map[string]any) {
	if h.store != nil {
		_ = h.store.AppendAudit(context.Background(), actor, action, target, details)
	}
}
func (h *APIHandler) activity(w http.ResponseWriter, r *http.Request) {
	v, e := h.store.Activity(r.Context())
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 200, v)
}

// turn runs one native CLI invocation. Conversation context lives in Mace, so each
// invocation receives the transcript and exits; no TUI or stdin prompt knowledge is needed.
func (h *APIHandler) turn(ctx context.Context, id, prompt string) (string, error) {
	h.lifecycleMu.Lock()
	locked := true
	defer func() {
		if locked {
			h.lifecycleMu.Unlock()
		}
	}()
	var s Session
	if e := h.store.Document(ctx, "session", id, &s); e != nil {
		return "", fmt.Errorf("agent session unavailable: %w", e)
	}
	if s.Status == "stopped" || s.Status == "terminated" || s.Status == "removed" {
		return "", fmt.Errorf("agent is %s; resume it before chatting", s.Status)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	prompt = agentInstructions(s) + "\n" + prompt
	h.sessionMu.Lock()
	if h.activeSessions[id] {
		h.sessionMu.Unlock()
		return "", errors.New("agent is already replying in another conversation")
	}
	h.activeSessions[id] = true
	h.sessionMu.Unlock()
	defer func() { h.sessionMu.Lock(); delete(h.activeSessions, id); h.sessionMu.Unlock() }()
	command := s.Provider
	var args []string
	switch s.Provider {
	case "codex":
		args = []string{"exec", "--json", "--skip-git-repo-check", prompt}
	case "claude":
		args = []string{"-p", "--output-format", "json", prompt}
	case "agy":
		args = []string{"--print", prompt, "--output-format", "text"}
	case "muse":
		args = []string{"exec", prompt}
	case "diagnostic":
		if os.Getenv("MACE_TEST_MODE") != "1" {
			return "", errors.New("diagnostic disabled")
		}
		command = "cat"
	default:
		return "", errors.New("unsupported agent adapter")
	}
	s.Status = "replying"
	h.store.PutDocument(context.Background(), "session", s.ID, s)
	defer func() {
		var current Session
		if h.store.Document(context.Background(), "session", s.ID, &current) == nil && current.Status == "replying" {
			current.Status = "ready"
			h.store.PutDocument(context.Background(), "session", s.ID, current)
		}
	}()
	proc, e := h.supervisor.StartAgent(ctx, protocol.AgentLaunchRequest{Name: s.Name + " reply", Command: command, Args: args, WorkingDir: s.Directory, TimeoutSeconds: 600, Labels: map[string]string{"session": id}})
	if e != nil {
		return "", e
	}
	h.audit(id, "TURN_STARTED", proc.ID, map[string]any{"session": id})
	if s.Provider == "diagnostic" {
		h.supervisor.SendInput(proc.ID, "Diagnostic reply. [GOAL_COMPLETE]\n")
	}
	h.supervisor.CloseInput(proc.ID)
	h.lifecycleMu.Unlock()
	locked = false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			h.supervisor.StopAgent(proc.ID, "SIGTERM", time.Second)
			return "", ctx.Err()
		case <-ticker.C:
			p, e := h.supervisor.GetAgent(proc.ID)
			if e != nil {
				return "", e
			}
			if !p.Status.IsTerminal() {
				continue
			}
			events, e := h.store.GetAgentEvents(ctx, proc.ID, 0, 10000)
			if e != nil {
				return "", e
			}
			var result, stderr strings.Builder
			for _, ev := range events {
				if ev.Type == protocol.EventStderr {
					stderr.WriteString(ev.Data)
					stderr.WriteByte('\n')
				}
				if ev.Type != protocol.EventStdout {
					continue
				}
				if s.Provider == "diagnostic" || s.Provider == "agy" || s.Provider == "muse" {
					result.WriteString(ev.Data)
					result.WriteByte('\n')
					continue
				}
				var obj map[string]any
				if json.Unmarshal([]byte(ev.Data), &obj) != nil {
					continue
				}
				if s.Provider == "claude" {
					if failed, ok := obj["is_error"].(bool); ok && failed {
						return "", fmt.Errorf("Claude Code: %v", obj["result"])
					}
					if text, ok := obj["result"].(string); ok {
						result.WriteString(text)
					}
				} else if obj["type"] == "item.completed" {
					if item, ok := obj["item"].(map[string]any); ok && item["type"] == "agent_message" {
						if text, ok := item["text"].(string); ok {
							result.WriteString(text)
							result.WriteByte('\n')
						}
					}
				}
			}
			if p.ExitCode != nil && *p.ExitCode != 0 {
				return "", fmt.Errorf("%s exited (%d): %s", s.Name, *p.ExitCode, truncate(stderr.String(), 3000))
			}
			if strings.TrimSpace(result.String()) == "" {
				return "", errors.New("agent returned no response; check host activity and provider login")
			}
			h.audit(id, "TURN_COMPLETED", proc.ID, map[string]any{"session": id})
			return strings.TrimSpace(result.String()), nil
		}
	}
}
func truncate(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
func (h *APIHandler) remoteTurn(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil || req.Prompt == "" {
		http.Error(w, "prompt required", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	reply, e := h.turn(ctx, routeParam(r, "id"), req.Prompt)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, 200, map[string]string{"text": reply})
}

type DesktopApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func desktopApps() []DesktopApp {
	home, _ := os.UserHomeDir()
	roots := []string{"/usr/share/applications", "/usr/local/share/applications", filepath.Join(home, ".local/share/applications")}
	out := []DesktopApp{}
	seen := map[string]bool{}
	for _, root := range roots {
		entries, _ := os.ReadDir(root)
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".desktop") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			b, e := os.ReadFile(path)
			if e != nil {
				continue
			}
			name := ""
			hidden := false
			in := false
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "[") {
					in = line == "[Desktop Entry]"
				}
				if !in {
					continue
				}
				if strings.HasPrefix(line, "Name=") {
					name = strings.TrimPrefix(line, "Name=")
				}
				if line == "NoDisplay=true" || line == "Hidden=true" {
					hidden = true
				}
			}
			if name != "" && !hidden && !seen[entry.Name()] {
				out = append(out, DesktopApp{ID: path, Name: name})
				seen[entry.Name()] = true
			}
		}
	}
	return out
}
func (h *APIHandler) apps(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, desktopApps()) }
func (h *APIHandler) hostAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		App  string `json:"app"`
		Path string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req) != nil {
		http.Error(w, "invalid action", 400)
		return
	}
	var command string
	var args []string
	name := "Open folder"
	if req.App != "" {
		found := false
		for _, a := range desktopApps() {
			if a.ID == req.App {
				found = true
				name = a.Name
			}
		}
		if !found {
			http.Error(w, "application not found", 404)
			return
		}
		command = "gio"
		args = []string{"launch", req.App}
	} else {
		absolute, e := filepath.Abs(req.Path)
		if e != nil {
			http.Error(w, e.Error(), 400)
			return
		}
		if _, e := os.Stat(absolute); e != nil {
			http.Error(w, "path unavailable", 400)
			return
		}
		command = "xdg-open"
		args = []string{absolute}
	}
	proc, e := h.supervisor.StartAgent(r.Context(), protocol.AgentLaunchRequest{Name: name, Command: command, Args: args})
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	h.supervisor.CloseInput(proc.ID)
	h.audit("you", "HOST_OPEN", proc.ID, map[string]any{"app": req.App, "path": req.Path})
	writeJSON(w, 201, proc)
}
