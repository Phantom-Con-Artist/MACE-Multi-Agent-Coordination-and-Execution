package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mace/internal/bootstrap"
)

// colorProfiles is the number of avatar color profiles the interface offers.
// Instances of the same agent get different ones by default.
const colorProfiles = 6

// profile serves POST /api/v1/sessions/{id}/profile. It renames an agent,
// changes its mention handle, sets the role it carries into every
// conversation, and picks its avatar color. Each field is optional.
func (h *APIHandler) profile(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	var req struct {
		Name   *string `json:"name"`
		Handle *string `json:"handle"`
		Role   *string `json:"role"`
		Color  *int    `json:"color"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req) != nil {
		http.Error(w, "Send a name, handle, role, or color.", 400)
		return
	}
	if peer, remoteID, remote := strings.Cut(id, "::"); remote {
		var p Peer
		if h.peerDoc(r.Context(), peer, &p) != nil {
			http.Error(w, "That host is not connected.", 404)
			return
		}
		var out Session
		if e := h.peerRequest(r.Context(), p, "/sessions/"+remoteID+"/profile", req, &out); e != nil {
			http.Error(w, e.Error(), 502)
			return
		}
		out.ID, out.Name = id, p.Name+" / "+out.Name
		h.store.PutDocument(r.Context(), "remote-session", id, out)
		writeJSON(w, 200, out)
		return
	}
	var current Session
	if h.store.Document(r.Context(), "session", id, &current) != nil {
		http.Error(w, "Agent not found.", 404)
		return
	}
	if req.Name != nil {
		if n := strings.TrimSpace(*req.Name); n == "" || len(n) > 60 {
			http.Error(w, "Use a name of 1 to 60 characters.", 400)
			return
		}
	}
	if req.Role != nil && len(*req.Role) > 2000 {
		http.Error(w, "Use a role under 2000 characters.", 400)
		return
	}
	if req.Color != nil && (*req.Color < 0 || *req.Color >= colorProfiles) {
		http.Error(w, "Unknown color.", 400)
		return
	}
	handle := ""
	if req.Handle != nil {
		handle = slug(*req.Handle)
		if handle != current.Handle && h.nextHandle(r.Context(), handle, id) != handle {
			http.Error(w, "@"+handle+" is already taken. Choose another mention name.", 409)
			return
		}
	}
	s, err := h.updateSession(r.Context(), id, func(s *Session) {
		if req.Name != nil {
			s.Name = strings.TrimSpace(*req.Name)
		}
		if handle != "" {
			s.Handle = handle
		}
		if req.Role != nil {
			s.Role = strings.TrimSpace(*req.Role)
		}
		if req.Color != nil {
			s.Color = req.Color
		}
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.audit("you", "AGENT_PROFILE_CHANGED", id, map[string]any{"name": s.Name, "handle": s.Handle, "role": s.Role})
	writeJSON(w, 200, s)
}

// avatarDir holds logos the owner provides, named after the provider, such as
// agy.svg or codex.png. Mace ships no marks it may not redistribute.
func avatarDir() string { return filepath.Join(bootstrap.DataDir(), "avatars") }

var avatarTypes = map[string]string{".svg": "image/svg+xml", ".png": "image/png", ".webp": "image/webp"}

func ownerAvatar(provider string) string {
	for ext := range avatarTypes {
		path := filepath.Join(avatarDir(), provider+ext)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() < 1<<20 {
			return path
		}
	}
	return ""
}

// avatar serves GET /api/v1/avatars/{provider}.
func (h *APIHandler) avatar(w http.ResponseWriter, r *http.Request) {
	provider := routeParam(r, "provider")
	if slug(provider) != provider {
		http.Error(w, "unknown provider", 404)
		return
	}
	path := ownerAvatar(provider)
	if path == "" {
		http.Error(w, "no logo provided", 404)
		return
	}
	w.Header().Set("Content-Type", avatarTypes[filepath.Ext(path)])
	// SVG files are untrusted: never let them run script in the page's origin.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}
