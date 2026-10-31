package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Private-chat commands. Each one changes or reports on the agent, and its
// result is posted to the private chat as a note: visible on every device,
// never sent to the agent. The agent learns about a new model or folder from
// its next awareness header.

type commandResult struct {
	Note    Message `json:"note"`
	Session Session `json:"session"`
}

type modelChoice struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// modelList reports where the choices came from: "cli" when the CLI listed
// them, "curated" for a known set of aliases, or "free" when only free entry
// is possible. Any model is checked by the CLI on the next turn.
type modelList struct {
	Current string        `json:"current"`
	Models  []modelChoice `json:"models"`
	Source  string        `json:"source"`
}

const commandHelp = `Commands in this chat:
/model <name>  Set the model everywhere this agent works ("default" for the CLI default)
/models  List models and show the current one
/cd <folder>  Change the working folder
/new  Start a fresh native session
/stop  Stop the current turn and clear waiting work
/status  Model, folder, waiting work, groups, and native session
/rename <handle>  Change the mention handle
/files  Open host files at the agent's folder
/help  Show this list`

var modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@\[\]-]{0,99}$`)

// command serves POST /api/v1/sessions/{id}/command.
func (h *APIHandler) command(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	var req struct {
		Name string `json:"name"`
		Arg  string `json:"arg"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
		http.Error(w, "Send a command name.", 400)
		return
	}
	req.Name, req.Arg = strings.TrimPrefix(strings.TrimSpace(req.Name), "/"), strings.TrimSpace(req.Arg)
	quiet := r.URL.Query().Get("quiet") == "1"

	if peer, remoteID, remote := strings.Cut(id, "::"); remote {
		var p Peer
		if h.peerDoc(r.Context(), peer, &p) != nil {
			http.Error(w, "That host is not connected.", 404)
			return
		}
		var out commandResult
		if e := h.peerRequest(r.Context(), p, "/sessions/"+remoteID+"/command?quiet=1", req, &out); e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		out.Session.ID, out.Session.Name = id, p.Name+" / "+out.Session.Name
		h.store.PutDocument(r.Context(), "remote-session", id, out.Session)
		out.Note = h.commandNote(id, out.Note.Text, false)
		writeJSON(w, 200, out)
		return
	}

	var s Session
	if h.store.Document(r.Context(), "session", id, &s) != nil {
		http.Error(w, "Agent not found.", 404)
		return
	}
	text, status, err := h.runCommand(r.Context(), &s, req.Name, req.Arg)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, 200, commandResult{Note: h.commandNote(id, text, quiet), Session: s})
}

func (h *APIHandler) runCommand(ctx context.Context, s *Session, name, arg string) (string, int, error) {
	update := func(change func(*Session)) error {
		next, err := h.updateSession(ctx, s.ID, change)
		*s = next
		return err
	}
	switch name {
	case "help":
		return commandHelp, 200, nil
	case "model":
		if arg == "" {
			return h.modelsText(ctx, *s), 200, nil
		}
		model := arg
		if strings.EqualFold(arg, "default") {
			model = ""
		} else if !modelPattern.MatchString(arg) {
			return "", 400, fmt.Errorf("%q is not a model name. Use letters, digits, and . _ - : /", limited(arg, 40))
		}
		if err := update(func(s *Session) { s.Model = model }); err != nil {
			return "", 500, err
		}
		h.audit("you", "AGENT_MODEL_CHANGED", s.ID, map[string]any{"model": model})
		if model == "" {
			return s.Name + " now uses the CLI default model, in every conversation.", 200, nil
		}
		return s.Name + " now uses " + model + " in every conversation. The CLI checks the name on the next turn.", 200, nil
	case "models":
		return h.modelsText(ctx, *s), 200, nil
	case "cd":
		if arg == "" {
			return "Folder: " + s.Directory, 200, nil
		}
		dir := arg
		if strings.HasPrefix(dir, "~") {
			home, _ := os.UserHomeDir()
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~"))
		} else if !filepath.IsAbs(dir) {
			dir = filepath.Join(s.Directory, dir)
		}
		dir = filepath.Clean(dir)
		if info, e := os.Stat(dir); e != nil || !info.IsDir() {
			return "", 400, fmt.Errorf("%s is not a folder on this host.", dir)
		}
		if err := update(func(s *Session) { s.Directory = dir }); err != nil {
			return "", 500, err
		}
		h.audit("you", "AGENT_FOLDER_CHANGED", s.ID, map[string]any{"directory": dir})
		return s.Name + " now works in " + dir + ". The next turn starts a fresh native session there.", 200, nil
	case "new":
		if s.Status == "replying" {
			return "", 409, fmt.Errorf("%s is replying. Use /stop first.", s.Name)
		}
		if err := update(func(s *Session) { s.NativeSession, s.NativeDir = "", "" }); err != nil {
			return "", 500, err
		}
		h.audit("you", "AGENT_NEW_SESSION", s.ID, nil)
		return "The next turn starts a fresh native session. Earlier messages stay here but " + s.Name + " will not remember its own past work.", 200, nil
	case "stop":
		waiting := h.waiting(s.ID)
		h.stopMember(s.ID)
		h.audit("you", "AGENT_TURN_STOPPED", s.ID, nil)
		if waiting == 0 {
			return s.Name + " had nothing running.", 200, nil
		}
		return fmt.Sprintf("Stopped %s and cleared %d waiting %s.", s.Name, waiting, plural(waiting, "turn", "turns")), 200, nil
	case "status":
		return h.statusText(ctx, *s), 200, nil
	case "rename":
		handle := slug(strings.TrimPrefix(arg, "@"))
		if arg == "" {
			return "Mention handle: @" + s.Handle, 200, nil
		}
		if handle != s.Handle && h.nextHandle(ctx, handle, s.ID) != handle {
			return "", 409, fmt.Errorf("@%s is already taken. Choose another mention name.", handle)
		}
		if err := update(func(s *Session) { s.Handle = handle }); err != nil {
			return "", 500, err
		}
		h.audit("you", "AGENT_PROFILE_CHANGED", s.ID, map[string]any{"handle": handle})
		return s.Name + " is now @" + handle + ".", 200, nil
	}
	return "", 400, fmt.Errorf("/%s is not a command here. Type /help for the list.", limited(name, 30))
}

// commandNote posts a command result to the private chat unless the call was
// proxied from another host, which posts it in its own copy of the chat.
func (h *APIHandler) commandNote(room, text string, quiet bool) Message {
	note := Message{ID: newID(), Room: room, Sender: "mace", Kind: "note", Text: text, At: time.Now().UTC(), Delivery: map[string]string{}}
	if !quiet {
		h.postMessage(context.Background(), &note)
	}
	return note
}

// waiting counts the running turn plus queued ones for an agent.
func (h *APIHandler) waiting(id string) int {
	h.sched.Lock()
	defer h.sched.Unlock()
	q := h.queues[id]
	if q == nil {
		return 0
	}
	n := len(q.queue)
	if q.running != nil {
		n++
	}
	return n
}

func (h *APIHandler) statusText(ctx context.Context, s Session) string {
	model := s.Model
	if model == "" {
		model = "CLI default"
	}
	native := "none yet, the next turn starts one"
	if s.NativeSession != "" && s.NativeDir == s.Directory {
		native = s.NativeSession
	} else if s.NativeSession != "" {
		native = "starts fresh on the next turn (folder changed)"
	}
	groups := []string{}
	raw, _ := h.store.Documents(ctx, "room")
	for _, b := range raw {
		var g Room
		if json.Unmarshal(b, &g) == nil && !g.Removed && containsID(g.Members, s.ID) {
			groups = append(groups, g.Name)
		}
	}
	inGroups := "none"
	if len(groups) > 0 {
		inGroups = strings.Join(groups, ", ")
	}
	n := h.waiting(s.ID)
	return fmt.Sprintf("%s (@%s) is %s.\nModel: %s\nFolder: %s\nWaiting: %d %s\nGroups: %s\nNative session: %s",
		s.Name, s.Handle, strings.ToLower(statusWord(s.Status)), model, s.Directory, n, plural(n, "turn", "turns"), inGroups, native)
}

func statusWord(status string) string {
	switch status {
	case "replying":
		return "Working"
	case "":
		return "Ready"
	}
	return status
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (h *APIHandler) modelsText(ctx context.Context, s Session) string {
	list := h.modelsFor(ctx, s)
	current := list.Current
	if current == "" {
		current = "CLI default"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Current model: %s", current)
	if len(list.Models) == 0 {
		b.WriteString("\nThis CLI does not list its models. Type /model <name> with any name it accepts.")
		return b.String()
	}
	b.WriteString("\nAvailable:")
	for _, m := range list.Models {
		fmt.Fprintf(&b, "\n%s", m.ID)
		if m.Label != "" && m.Label != m.ID {
			fmt.Fprintf(&b, "  %s", m.Label)
		}
	}
	return b.String()
}

// models serves GET /api/v1/sessions/{id}/models.
func (h *APIHandler) models(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	if peer, remoteID, remote := strings.Cut(id, "::"); remote {
		var p Peer
		if h.peerDoc(r.Context(), peer, &p) != nil {
			http.Error(w, "That host is not connected.", 404)
			return
		}
		var out modelList
		if e := h.peerRequest(r.Context(), p, "/sessions/"+remoteID+"/models", nil, &out); e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var s Session
	if h.store.Document(r.Context(), "session", id, &s) != nil {
		http.Error(w, "Agent not found.", 404)
		return
	}
	writeJSON(w, 200, h.modelsFor(r.Context(), s))
}

func (h *APIHandler) modelsFor(ctx context.Context, s Session) modelList {
	models, source, _ := listModels(ctx, s.Provider)
	if models == nil {
		models = []modelChoice{}
	}
	return modelList{Current: s.Model, Models: models, Source: source}
}

// listModels is replaced in tests.
var listModels = providerModels

var modelCache = struct {
	sync.Mutex
	at   map[string]time.Time
	list map[string][]modelChoice
}{at: map[string]time.Time{}, list: map[string][]modelChoice{}}

// providerModels asks the CLI for its models where it offers a list (agy
// models), uses documented aliases for Claude Code, and otherwise leaves the
// choice to free entry. CLI lists are cached for ten minutes.
func providerModels(ctx context.Context, provider string) ([]modelChoice, string, error) {
	switch provider {
	case "claude":
		return []modelChoice{{ID: "opus", Label: "Opus, latest"}, {ID: "sonnet", Label: "Sonnet, latest"}, {ID: "haiku", Label: "Haiku, latest"}}, "curated", nil
	case "agy":
	default:
		return nil, "free", nil
	}
	modelCache.Lock()
	if time.Since(modelCache.at[provider]) < 10*time.Minute {
		list := modelCache.list[provider]
		modelCache.Unlock()
		return list, "cli", nil
	}
	modelCache.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, provider, "models").Output()
	if err != nil {
		return nil, "free", err
	}
	list := parseModelLines(string(out))
	modelCache.Lock()
	modelCache.at[provider], modelCache.list[provider] = time.Now(), list
	modelCache.Unlock()
	return list, "cli", nil
}

// parseModelLines reads "id<TAB>label" lines and skips anything else, such
// as a "Fetching available models..." banner.
func parseModelLines(out string) []modelChoice {
	list := []modelChoice{}
	for _, line := range strings.Split(out, "\n") {
		id, label, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if ok && modelPattern.MatchString(id) {
			list = append(list, modelChoice{ID: id, Label: strings.TrimSpace(label)})
		}
	}
	return list
}
