package api

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Activity is one visible step of an agent turn, such as reading or editing a
// file or running a command, shown live while the agent works and kept with
// its reply.
type Activity struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"` // read, edit, command, search, web, browser, think, tool
	Title  string    `json:"title"`
	Detail string    `json:"detail,omitempty"`
	State  string    `json:"state"` // running, done, failed
	At     time.Time `json:"at"`
}

// turnState accumulates what an adapter learns from a CLI's output stream.
type turnState struct {
	SessionID string
	Final     string
	HaveFinal bool
	Deltas    strings.Builder
	Err       string
	Steps     []Activity
	index     map[string]int
	changed   bool
}

// pathInTicks matches a backticked absolute path, which CLIs often print in
// full. A phone shows the file name; the full path stays in the detail.
var pathInTicks = regexp.MustCompile("`(/[^`\\s]+)`")

func shortTitle(title string) string {
	title = strings.Replace(title, "Read text file ", "Read ", 1)
	return pathInTicks.ReplaceAllStringFunc(title, func(m string) string {
		return "`" + filepath.Base(strings.Trim(m, "`")) + "`"
	})
}

func (t *turnState) step(id, kind, title, detail, state string) {
	if t.index == nil {
		t.index = map[string]int{}
	}
	title = shortTitle(title)
	detail = limited(strings.TrimSpace(detail), 400)
	if i, ok := t.index[id]; ok {
		s := &t.Steps[i]
		if title != "" {
			s.Title, s.Kind = title, kind
		}
		if detail != "" {
			s.Detail = detail
		}
		s.State = state
	} else {
		if len(t.Steps) >= 300 {
			return
		}
		t.index[id] = len(t.Steps)
		t.Steps = append(t.Steps, Activity{ID: id, Kind: kind, Title: title, Detail: detail, State: state, At: time.Now().UTC()})
	}
	t.changed = true
}

// reply is the agent's final text: the CLI's explicit result when it reports
// one, otherwise the streamed text.
func (t *turnState) reply() string {
	if t.HaveFinal {
		return strings.TrimSpace(t.Final)
	}
	return strings.TrimSpace(t.Deltas.String())
}

type adapter struct {
	// chooseID means Mace picks the native session ID before the first turn.
	chooseID bool
	// streams means the CLI reports progress while working, so a silent process
	// can be treated as stuck.
	streams bool
	args    func(s Session, prompt, nativeID string, resume bool) []string
	parse   func(line string, t *turnState)
	// resumeMissing recognizes an error caused by a native session that no
	// longer exists, so the turn can be retried in a fresh session.
	resumeMissing func(stderr string) bool
}

func withModel(args []string, s Session) []string {
	if s.Model != "" {
		return append([]string{"--model", s.Model}, args...)
	}
	return args
}

var adapters = map[string]adapter{
	// Verified live on 2026-10-31: stream-json, conversation_id, --conversation.
	// An unknown ID starts a new conversation instead of failing.
	"agy": {
		streams: true,
		args: func(s Session, prompt, id string, resume bool) []string {
			a := []string{"--print", prompt, "--output-format", "stream-json"}
			if resume {
				a = append(a, "--conversation", id)
			}
			return withModel(a, s)
		},
		parse: parseAgy,
	},
	// Verified live on 2026-10-31: exec --json, --session-id creates or resumes.
	"muse": {
		chooseID: true,
		streams:  true,
		args: func(s Session, prompt, id string, resume bool) []string {
			return withModel([]string{"exec", "--json", "--session-id", id, prompt}, s)
		},
		parse: parseMuse,
	},
	// From Claude Code's documented stream-json format; not verified live.
	"claude": {
		chooseID: true,
		streams:  true,
		args: func(s Session, prompt, id string, resume bool) []string {
			a := []string{"-p", "--output-format", "stream-json", "--verbose"}
			if resume {
				a = append(a, "--resume", id)
			} else {
				a = append(a, "--session-id", id)
			}
			return withModel(append(a, prompt), s)
		},
		parse:         parseClaude,
		resumeMissing: func(e string) bool { return strings.Contains(strings.ToLower(e), "no conversation found") },
	},
	// From Codex's documented exec --json format; not verified live.
	"codex": {
		streams: true,
		args: func(s Session, prompt, id string, resume bool) []string {
			a := []string{"exec", "--json", "--skip-git-repo-check"}
			if s.Model != "" {
				a = append(a, "--model", s.Model)
			}
			if resume {
				return append(a, "resume", id, prompt)
			}
			return append(a, prompt)
		},
		parse: parseCodex,
		resumeMissing: func(e string) bool {
			e = strings.ToLower(e)
			return strings.Contains(e, "session") && (strings.Contains(e, "not found") || strings.Contains(e, "no such"))
		},
	},
	"diagnostic": {
		args:  func(Session, string, string, bool) []string { return nil },
		parse: func(line string, t *turnState) { t.Deltas.WriteString(line + "\n") },
	},
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func obj(v any) map[string]any { m, _ := v.(map[string]any); return m }

// describeTool turns a native tool call into a step a person can read at a
// glance, in the style of the CLIs' own transcripts.
func describeTool(name string, params map[string]any) (kind, title, detail string) {
	n := strings.ToLower(name)
	path := str(params, "file_path", "path", "Path", "AbsolutePath", "TargetFile", "notebook_path", "DirectoryPath", "SearchPath")
	short := filepath.Base(path)
	if path == "" || short == "." || short == "/" {
		short = path
	}
	command := str(params, "command", "CommandLine", "cmd")
	query := str(params, "pattern", "query", "Query", "Pattern", "SearchPattern")
	url := str(params, "url", "Url", "URL")
	switch {
	case command != "" || strings.Contains(n, "command") || n == "bash" || n == "shell":
		return "command", "Ran " + "`" + limited(firstLine(command), 120) + "`", command
	case strings.Contains(n, "write") || strings.Contains(n, "edit") || strings.Contains(n, "replace") || strings.Contains(n, "sed") || strings.Contains(n, "patch") || strings.Contains(n, "notebook_edit"):
		if short == "" {
			return "edit", "Edited files", ""
		}
		return "edit", "Edited `" + short + "`", path
	case strings.Contains(n, "list") || strings.Contains(n, "glob") || strings.Contains(n, "find"):
		target := short
		if query != "" {
			target = query
		}
		return "search", strings.TrimSpace("Listed " + "`" + target + "`"), path
	case strings.Contains(n, "grep") || strings.Contains(n, "search") && !strings.Contains(n, "web"):
		return "search", "Searched for `" + limited(query, 80) + "`", path
	case strings.Contains(n, "web") || strings.Contains(n, "url") || strings.Contains(n, "fetch"):
		if query != "" {
			return "web", "Searched the web for `" + limited(query, 80) + "`", ""
		}
		return "web", "Read " + limited(url, 120), url
	case strings.HasPrefix(n, "browser") || strings.Contains(n, "browser"):
		return "browser", "Used the browser: " + strings.ReplaceAll(strings.TrimPrefix(n, "browser_"), "_", " "), url
	case strings.Contains(n, "read") || strings.Contains(n, "view") || n == "cat":
		if short == "" {
			return "read", "Read a file", ""
		}
		return "read", "Read `" + short + "`", path
	}
	return "tool", "Used " + strings.ReplaceAll(name, "_", " "), ""
}

func firstLine(s string) string { line, _, _ := strings.Cut(strings.TrimSpace(s), "\n"); return line }

func parseAgy(line string, t *turnState) {
	var ev map[string]any
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	switch ev["event"] {
	case "init":
		t.SessionID = str(ev, "conversation_id")
	case "step_update":
		u := obj(ev["step_update"])
		id := fmt.Sprint(u["step_index"])
		switch u["step_type"] {
		case "tool":
			info := obj(u["tool_info"])
			kind, title, detail := describeTool(str(u, "tool_name"), obj(info["parameters"]))
			state := "running"
			if u["state"] == "DONE" {
				state = "done"
			} else if u["state"] == "ERROR" || u["state"] == "FAILED" {
				state = "failed"
			}
			if out := str(info, "output"); out != "" && state != "running" {
				detail = out
			}
			t.step("agy-"+id, kind, title, detail, state)
		case "agent_response":
			t.Deltas.WriteString(str(u, "text_delta"))
		}
	case "result":
		r := obj(ev["result"])
		if id := str(r, "conversation_id"); id != "" {
			t.SessionID = id
		}
		if status := str(r, "status"); status != "" && status != "SUCCESS" {
			t.Err = "Antigravity reported " + strings.ToLower(status) + ": " + limited(str(r, "error", "message", "response"), 500)
		}
		t.Final, t.HaveFinal = str(r, "response"), true
	}
}

func parseMuse(line string, t *turnState) {
	var ev map[string]any
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	if s := obj(ev["stream"]); s["kind"] == "session" && t.SessionID == "" {
		t.SessionID = str(s, "id")
	}
	p := obj(ev["payload"])
	typ := str(ev, "payload_type")
	switch {
	case typ == "run.output.delta":
		t.Deltas.WriteString(str(p, "text"))
	case strings.HasPrefix(typ, "run.terminal."):
		if p["terminal"] == "completed" {
			t.Final, t.HaveFinal = str(p, "text"), true
		} else {
			t.Err = "Muse run " + str(p, "terminal") + ": " + limited(fmt.Sprint(p["reason"]), 500)
		}
	case strings.HasPrefix(typ, "task.lifecycle."):
		e := obj(p["event"])
		id := str(e, "task_id")
		switch str(e, "kind") {
		case "proposed":
			if tool, ok := strings.CutPrefix(str(e, "task_kind"), "tool."); ok {
				kind, title, _ := describeTool(tool, nil)
				t.step(id, kind, title, "", "running")
			}
		case "output":
			if _, known := t.index[id]; known {
				summary := firstLine(str(e, "chunk"))
				t.step(id, t.Steps[t.index[id]].Kind, strings.TrimSuffix(summary, "."), str(e, "chunk"), "running")
			}
		case "completed", "failed", "cancelled":
			if i, known := t.index[id]; known {
				state := "done"
				if str(e, "kind") != "completed" {
					state = "failed"
				}
				t.step(id, t.Steps[i].Kind, "", "", state)
			}
		}
	}
}

func parseClaude(line string, t *turnState) {
	var ev map[string]any
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	if id := str(ev, "session_id"); id != "" {
		t.SessionID = id
	}
	switch ev["type"] {
	case "assistant":
		for _, c := range anySlice(obj(ev["message"])["content"]) {
			block := obj(c)
			switch block["type"] {
			case "tool_use":
				kind, title, detail := describeTool(str(block, "name"), obj(block["input"]))
				t.step(str(block, "id"), kind, title, detail, "running")
			case "text":
				t.Deltas.WriteString(str(block, "text"))
			}
		}
	case "user":
		for _, c := range anySlice(obj(ev["message"])["content"]) {
			block := obj(c)
			if block["type"] == "tool_result" {
				id := str(block, "tool_use_id")
				if i, ok := t.index[id]; ok {
					state := "done"
					if failed, _ := block["is_error"].(bool); failed {
						state = "failed"
					}
					t.step(id, t.Steps[i].Kind, "", "", state)
				}
			}
		}
	case "result":
		if failed, _ := ev["is_error"].(bool); failed {
			t.Err = "Claude Code: " + limited(str(ev, "result", "error"), 500)
		}
		t.Final, t.HaveFinal = str(ev, "result"), true
	}
}

func parseCodex(line string, t *turnState) {
	var ev map[string]any
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	switch ev["type"] {
	case "thread.started":
		t.SessionID = str(ev, "thread_id")
	case "item.started", "item.updated", "item.completed":
		item := obj(ev["item"])
		id := str(item, "id")
		state := "running"
		if ev["type"] == "item.completed" {
			state = "done"
			if item["status"] == "failed" {
				state = "failed"
			}
		}
		switch item["type"] {
		case "agent_message":
			if ev["type"] == "item.completed" {
				t.Deltas.WriteString(str(item, "text") + "\n")
			}
		case "command_execution":
			kind, title, detail := describeTool("command", map[string]any{"command": str(item, "command")})
			t.step(id, kind, title, detail, state)
		case "file_change":
			files := []string{}
			for _, c := range anySlice(item["changes"]) {
				files = append(files, filepath.Base(str(obj(c), "path")))
			}
			t.step(id, "edit", "Edited "+limited("`"+strings.Join(files, "`, `")+"`", 160), "", state)
		case "web_search":
			kind, title, detail := describeTool("web_search", map[string]any{"query": str(item, "query")})
			t.step(id, kind, title, detail, state)
		case "mcp_tool_call":
			kind, title, detail := describeTool(str(item, "tool"), obj(item["arguments"]))
			t.step(id, kind, title, detail, state)
		}
	case "turn.failed", "error":
		t.Err = "Codex: " + limited(str(obj(ev["error"]), "message")+str(ev, "message"), 500)
	}
}

func anySlice(v any) []any { s, _ := v.([]any); return s }
