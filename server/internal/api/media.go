package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Phones send pictures, videos, and documents into a conversation the way
// a messenger does: the app uploads the file, the hub keeps it with the
// conversation's shares, and the app posts a message pointing at it. Agents
// open it like any other shared file. Agents can have their own photos.

const maxUpload = 200 << 20

const maxPhoto = 2 << 20

var uploadUnsafe = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

// upload serves POST /api/v1/chats/{room}/upload?name=<file name> with the
// file as the body. It answers {"file": path} for the message that follows.
func (h *APIHandler) upload(w http.ResponseWriter, r *http.Request) {
	room := routeParam(r, "room")
	if slug(room) == "" || strings.ContainsAny(room, `/\`) {
		http.Error(w, "unknown conversation", 404)
		return
	}
	name := strings.TrimSpace(uploadUnsafe.ReplaceAllString(filepath.Base(r.URL.Query().Get("name")), "-"))
	if name == "" || name == "." || name == ".." {
		name = "file"
	}
	dir := filepath.Join(h.sharesDir(strings.ReplaceAll(room, "::", "--")), "uploads")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+limited(name, 120))
	rel, inside := h.rootRelative(path)
	if !inside {
		http.Error(w, "The hub's storage is outside its shared folder.", 500)
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	n, err := io.Copy(f, http.MaxBytesReader(w, r.Body, maxUpload))
	f.Close()
	if err != nil || n == 0 {
		os.Remove(path)
		http.Error(w, fmt.Sprintf("Upload failed. Files up to %d MB can be sent.", maxUpload>>20), 400)
		return
	}
	h.audit("you", "FILE_UPLOADED", room, map[string]any{"file": rel, "bytes": n})
	writeJSON(w, 201, map[string]any{"file": rel, "size": n})
}

var photoTypes = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

func photoPath(id string) string {
	for _, ext := range photoTypes {
		p := filepath.Join(avatarDir(), "agent-"+photoKey(id)+ext)
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

func photoKey(id string) string { return uploadUnsafe.ReplaceAllString(strings.ReplaceAll(id, "::", "--"), "-") }

// photo serves GET and POST /api/v1/sessions/{id}/photo: an agent's own
// picture, PNG, JPEG, or WebP. DELETE removes it. Groups have the same
// at /rooms/{room}/photo.
func (h *APIHandler) photo(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	if id == "" {
		id = routeParam(r, "room")
	}
	switch r.Method {
	case "GET":
		p := photoPath(id)
		if p == "" {
			http.Error(w, "no photo", 404)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=60")
		http.ServeFile(w, r, p)
	case "DELETE":
		if p := photoPath(id); p != "" {
			os.Remove(p)
		}
		h.events.publish("session", map[string]any{"id": id})
		w.WriteHeader(204)
	default:
		var room Room
		if !h.knownMember(r.Context(), id) && h.store.Document(r.Context(), "room", id, &room) != nil {
			http.Error(w, "unknown agent or group", 404)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPhoto))
		if err != nil || len(data) == 0 {
			http.Error(w, "Send a picture up to 2 MB.", 400)
			return
		}
		ext, ok := photoTypes[http.DetectContentType(data)]
		if !ok {
			http.Error(w, "Send a PNG, JPEG, or WebP picture.", 400)
			return
		}
		if err := os.MkdirAll(avatarDir(), 0o700); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if old := photoPath(id); old != "" {
			os.Remove(old)
		}
		if err := os.WriteFile(filepath.Join(avatarDir(), "agent-"+photoKey(id)+ext), data, 0o600); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		h.audit("you", "AGENT_PHOTO", id, nil)
		h.events.publish("session", map[string]any{"id": id})
		writeJSON(w, 200, map[string]any{"photo": time.Now().Unix()})
	}
}

func photoTime(id string) int64 {
	if p := photoPath(id); p != "" {
		if info, err := os.Stat(p); err == nil {
			return info.ModTime().Unix()
		}
	}
	return 0
}
