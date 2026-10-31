package api

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Agents working in a repository share a code graph built by graphify
// (github.com/Graphify-Labs/graphify), so they ask where a symbol lives and
// who calls it instead of grepping and reading whole files. Mace only runs
// graphify's code pass, which parses with tree-sitter and never calls a
// model, so keeping the graph fresh costs no quota. Without graphify
// installed, nothing changes.

// graphRefreshGap is the shortest time between rebuilds of one folder.
const graphRefreshGap = 30 * time.Second

type codeGraphs struct {
	mu      sync.Mutex
	running map[string]bool
	pending map[string]bool
	last    map[string]time.Time
	// command overrides the graphify executable, for tests.
	command string
}

func (g *codeGraphs) executable() string {
	if g.command != "" {
		return g.command
	}
	if os.Getenv("MACE_TEST_MODE") == "1" {
		return ""
	}
	p, _ := exec.LookPath("graphify")
	return p
}

// graphable reports whether a folder is a repository worth indexing. The
// filesystem root and the home folder never are.
func graphable(dir string) bool {
	if dir == "" || dir == "/" {
		return false
	}
	if home, _ := os.UserHomeDir(); filepath.Clean(dir) == filepath.Clean(home) {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && info.IsDir()
}

func graphFile(dir string) string { return filepath.Join(dir, "graphify-out", "graph.json") }

// refresh rebuilds a folder's code graph in the background. Calls while a
// rebuild runs, or within graphRefreshGap of the last one, fold into one
// rebuild once the gap has passed.
func (g *codeGraphs) refresh(dir string) {
	bin := g.executable()
	if bin == "" || !graphable(dir) {
		return
	}
	g.mu.Lock()
	if g.running == nil {
		g.running, g.pending, g.last = map[string]bool{}, map[string]bool{}, map[string]time.Time{}
	}
	if g.running[dir] || g.pending[dir] {
		g.pending[dir] = true
		g.mu.Unlock()
		return
	}
	if wait := graphRefreshGap - time.Since(g.last[dir]); wait > 0 {
		g.pending[dir] = true
		g.mu.Unlock()
		time.AfterFunc(wait, func() { g.retry(dir) })
		return
	}
	g.running[dir] = true
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			g.running[dir], g.last[dir] = false, time.Now()
			again := g.pending[dir]
			g.mu.Unlock()
			if again {
				time.AfterFunc(graphRefreshGap, func() { g.retry(dir) })
			}
		}()
		excludeGraphOutput(dir)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "update", dir)
		cmd.Dir = dir
		cmd.Env = append(graphEnv(), "GRAPHIFY_BACKEND=none")
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("code graph for %s: %v: %s", dir, err, truncate(string(out), 400))
		}
	}()
}

func (g *codeGraphs) retry(dir string) {
	g.mu.Lock()
	g.pending[dir] = false
	g.mu.Unlock()
	g.refresh(dir)
}

func graphEnv() []string {
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "MACE_") && !strings.HasPrefix(kv, "GRAPHIFY_BACKEND=") {
			env = append(env, kv)
		}
	}
	return env
}

// excludeGraphOutput keeps graphify-out out of git status through the
// repository's private exclude file, so the owner's .gitignore is untouched.
func excludeGraphOutput(dir string) {
	path := filepath.Join(dir, ".git", "info", "exclude")
	b, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(b), "\n") {
		if t := strings.TrimSpace(line); t == "graphify-out/" || t == "/graphify-out/" || t == "graphify-out" {
			return
		}
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		f.WriteString("\n")
	}
	f.WriteString("graphify-out/\n")
}

// codeMapLine tells an agent how to use its folder's code graph.
func (h *APIHandler) codeMapLine(ctx context.Context, member string) string {
	if strings.Contains(member, "::") {
		return ""
	}
	var s Session
	if h.store.Document(ctx, "session", member, &s) != nil || !graphable(s.Directory) {
		return ""
	}
	if _, err := os.Stat(graphFile(s.Directory)); err != nil {
		h.graphs.refresh(s.Directory)
		return ""
	}
	return "CODE MAP: your folder has a code graph. Before grepping or opening many files, ask it: graphify explain <symbol>, graphify path <A> <B>, or graphify query \"<question>\" --budget 1500. Then open only the files it points to. Mace refreshes it after each turn.\n"
}

// refreshAfterTurn rebuilds the graph of the folder a local agent just worked in.
func (h *APIHandler) refreshAfterTurn(ctx context.Context, member string) {
	if strings.Contains(member, "::") {
		return
	}
	var s Session
	if h.store.Document(ctx, "session", member, &s) == nil {
		if _, err := os.Stat(graphFile(s.Directory)); err == nil {
			h.graphs.refresh(s.Directory)
		}
	}
}
