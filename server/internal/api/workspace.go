package api

import (
	"context"
	"encoding/json"
	"fmt"
	"mace/pkg/protocol"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Session struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Handle   string `json:"handle,omitempty"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	// Role is the agent's own role. It applies in every conversation unless a
	// group assigns a role of its own.
	Role string `json:"role,omitempty"`
	// Color picks the avatar's color profile; nil means chosen by instance.
	Color *int `json:"color,omitempty"`
	// Avatar reports a logo the owner provided for this agent's provider.
	Avatar    bool   `json:"avatar,omitempty"`
	// Photo is when the agent's own picture last changed, as Unix seconds;
	// 0 when it has none. Apps load it from /sessions/{id}/photo.
	Photo     int64  `json:"photo,omitempty"`
	Directory string `json:"working_dir"`
	Status    string `json:"status"`
	// NativeSession is the CLI's own session ID, shared by every conversation
	// this coworker is in. NativeDir is the folder it was created in; CLIs that
	// key sessions by folder start fresh when the folder changes.
	NativeSession string `json:"native_session,omitempty"`
	NativeDir     string `json:"native_dir,omitempty"`
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
		out := make([]Session, 0, len(v))
		for _, b := range v {
			var s Session
			if json.Unmarshal(b, &s) == nil {
				s.Avatar = ownerAvatar(s.Provider) != ""
				s.Photo = photoTime(s.ID)
				out = append(out, s)
			}
		}
		writeJSON(w, 200, out)
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
	s.Handle = h.nextHandle(r.Context(), s.Provider, "")
	s.NativeSession, s.NativeDir = "", ""
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

type DesktopApp struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
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
			name, category := "", ""
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
				if strings.HasPrefix(line, "Categories=") {
					category = mainCategory(strings.TrimPrefix(line, "Categories="))
				}
				if line == "NoDisplay=true" || line == "Hidden=true" {
					hidden = true
				}
			}
			if name != "" && !hidden && !seen[entry.Name()] {
				out = append(out, DesktopApp{ID: path, Name: name, Category: category})
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

// mainCategory turns a desktop entry's Categories list into one of the
// freedesktop main categories, named for people, or "Other".
func mainCategory(list string) string {
	names := map[string]string{
		"AudioVideo": "Media", "Audio": "Media", "Video": "Media",
		"Development": "Development", "Education": "Education", "Game": "Games",
		"Graphics": "Graphics", "Network": "Internet", "Office": "Office",
		"Science": "Science", "Settings": "Settings", "System": "System", "Utility": "Utilities",
	}
	for _, c := range strings.Split(list, ";") {
		if n, ok := names[strings.TrimSpace(c)]; ok {
			return n
		}
	}
	return "Other"
}
