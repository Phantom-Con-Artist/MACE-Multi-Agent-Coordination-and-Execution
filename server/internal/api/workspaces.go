package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"mace/internal/storage"

	"github.com/go-chi/chi/v5"
)

// Sessions: the owner starts a new session to begin with nothing, and
// earlier ones stay saved until removed. Each session has its own database
// (internal/storage/workspaces.go), and switching restarts the daemon so
// nothing in memory carries over. Linked machines follow: the machine that
// switched tells the others, and a ping settles any that missed it, so the
// same session runs everywhere. In code a session is a "workspace", since
// "session" already names an agent.

// workspaceIDs are what a session ID may look like; IDs from other machines
// become file names, so nothing else is accepted.
var workspaceIDs = regexp.MustCompile(`^(default|w[0-9a-f]{12})$`)

// workspaceSwitch tells a linked machine which session to run.
type workspaceSwitch struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	At   time.Time `json:"at"`
}

type workspaceView struct {
	storage.Workspace
	Current bool `json:"current"`
}

func (h *APIHandler) currentWorkspace(ctx context.Context) *workspaceSwitch {
	if h.dataDir == "" {
		return nil
	}
	w, err := h.store.ActiveWorkspace(ctx)
	if err != nil {
		return nil
	}
	return &workspaceSwitch{ID: w.ID, Name: w.Name, At: h.store.ActiveSince(ctx).At}
}

func newWorkspaceID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return "w" + hex.EncodeToString(b)
}

// workspaces serves GET /api/v1/workspaces (the sessions, the current one
// marked) and POST /api/v1/workspaces {name} (start a new session and switch
// to it).
func (h *APIHandler) workspaces(w http.ResponseWriter, r *http.Request) {
	if h.dataDir == "" {
		http.Error(w, "Sessions are not available on this MACE.", 501)
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodPost {
		var body struct {
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		name := limited(strings.TrimSpace(body.Name), 60)
		if name == "" {
			name = "Session " + time.Now().Format("Jan 2 15:04")
		}
		ws, err := h.store.AddWorkspace(ctx, newWorkspaceID(), name)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		h.enterWorkspace(ctx, workspaceSwitch{ID: ws.ID, Name: ws.Name, At: time.Now().UTC()}, true)
		writeJSON(w, 200, ws)
		return
	}
	list, err := h.store.Workspaces(ctx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	current := h.store.ActiveSince(ctx).ID
	out := []workspaceView{}
	for _, ws := range list {
		out = append(out, workspaceView{Workspace: ws, Current: ws.ID == current})
	}
	writeJSON(w, 200, out)
}

// switchWorkspace serves POST /api/v1/workspaces/{id}/switch.
func (h *APIHandler) switchWorkspace(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var ws storage.Workspace
	if h.dataDir == "" || h.store.Document(ctx, "workspace", chi.URLParam(r, "id"), &ws) != nil {
		http.Error(w, "There is no such session.", 404)
		return
	}
	if h.store.ActiveSince(ctx).ID == ws.ID {
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	h.enterWorkspace(ctx, workspaceSwitch{ID: ws.ID, Name: ws.Name, At: time.Now().UTC()}, true)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// removeWorkspace serves POST /api/v1/workspaces/{id}/remove: the session's
// database and its groups' shared files are deleted on this machine.
func (h *APIHandler) removeWorkspace(w http.ResponseWriter, r *http.Request) {
	if h.dataDir == "" {
		http.Error(w, "Sessions are not available on this MACE.", 501)
		return
	}
	id := chi.URLParam(r, "id")
	rooms, err := h.store.RemoveWorkspace(r.Context(), h.dataDir, id)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	for _, room := range rooms {
		if room != "" && !strings.ContainsAny(room, "/\\.") {
			os.RemoveAll(h.sharesDir(room))
		}
	}
	h.audit("you", "SESSION_REMOVED", id, map[string]any{"groups": len(rooms)})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// enterWorkspace makes a session the active one and restarts into it. The
// machine where the owner chose it tells every linked machine to follow.
func (h *APIHandler) enterWorkspace(ctx context.Context, ws workspaceSwitch, tell bool) {
	if err := h.store.SetActiveWorkspace(ctx, ws.ID, ws.At); err != nil {
		log.Printf("[MACE] Session %s: %v", ws.ID, err)
		return
	}
	h.audit("you", "SESSION_ACTIVE", ws.ID, map[string]any{"name": ws.Name})
	h.events.publish("workspace", ws)
	if tell {
		for _, n := range h.knownNodes(ctx) {
			if n.ID == h.selfID() {
				continue
			}
			c, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			if err := h.nodeRequest(c, n, "POST", "/api/v1/node/workspace", ws, nil); err != nil {
				log.Printf("[MACE] Session switch to %s: %v (it follows on its next ping)", n.Name, err)
			}
			cancel()
		}
	}
	if h.restart != nil {
		// Let this answer reach the app first.
		time.AfterFunc(600*time.Millisecond, h.restart)
	}
}

// followWorkspace switches to a session a linked machine runs, if it was
// chosen after this machine's own; a new session is created with the same ID.
func (h *APIHandler) followWorkspace(ctx context.Context, ws workspaceSwitch) {
	if h.dataDir == "" || !workspaceIDs.MatchString(ws.ID) {
		return
	}
	ws.Name = limited(ws.Name, 60)
	mine := h.store.ActiveSince(ctx)
	if mine.ID == ws.ID || !ws.At.After(mine.At) {
		return
	}
	var known storage.Workspace
	if h.store.Document(ctx, "workspace", ws.ID, &known) != nil {
		if _, err := h.store.AddWorkspace(ctx, ws.ID, ws.Name); err != nil {
			return
		}
	}
	log.Printf("[MACE] Following a linked machine into session %q", ws.Name)
	h.enterWorkspace(ctx, ws, false)
}

// nodeWorkspace serves POST /api/v1/node/workspace: a linked machine switched
// sessions.
func (h *APIHandler) nodeWorkspace(w http.ResponseWriter, r *http.Request) {
	_, body, err := h.verifyNodeRequest(r)
	if err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	var ws workspaceSwitch
	if json.Unmarshal(body, &ws) != nil || !workspaceIDs.MatchString(ws.ID) {
		http.Error(w, "bad session", 400)
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
	go h.followWorkspace(context.Background(), ws)
}
