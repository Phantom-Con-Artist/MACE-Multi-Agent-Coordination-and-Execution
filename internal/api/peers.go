package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Peer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Token string `json:"token,omitempty"`
}

func (h *APIHandler) peerRequest(ctx context.Context, p Peer, path string, body any, result any) error {
	var data []byte
	var e error
	method := "GET"
	if body != nil {
		method = "POST"
		data, e = json.Marshal(body)
		if e != nil {
			return e
		}
	}
	req, e := http.NewRequestWithContext(ctx, method, p.URL+"/api/v1"+path, bytes.NewReader(data))
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	req.Header.Set("Content-Type", "application/json")
	timeout := 55 * time.Second
	if strings.HasSuffix(path, "/turn") {
		timeout = 11 * time.Minute
	}
	client := http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, e := client.Do(req)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		return fmt.Errorf("peer returned %d: %s", res.StatusCode, b)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 48<<20)).Decode(result)
}
func (h *APIHandler) peers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		raw, e := h.store.Documents(r.Context(), "peer")
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		out := []Peer{}
		for _, b := range raw {
			var p Peer
			json.Unmarshal(b, &p)
			p.Token = ""
			out = append(out, p)
		}
		writeJSON(w, 200, out)
		return
	}
	var p Peer
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&p) != nil {
		http.Error(w, "invalid peer", 400)
		return
	}
	u, e := url.Parse(strings.TrimRight(p.URL, "/"))
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || p.Token == "" {
		http.Error(w, "use a host address and its access token", 400)
		return
	}
	p.URL = u.String()
	var info map[string]any
	if e := h.peerRequest(r.Context(), p, "/node", nil, &info); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	p.ID = "peer_" + newID()
	if p.Name == "" {
		p.Name = fmt.Sprint(info["name"])
	}
	if e := h.store.PutDocument(r.Context(), "peer", p.ID, p); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	h.audit("you", "PEER_CONNECTED", p.ID, map[string]any{"url": p.URL})
	p.Token = ""
	writeJSON(w, 201, p)
}
func (h *APIHandler) peerSessions(w http.ResponseWriter, r *http.Request) {
	var p Peer
	if h.store.Document(r.Context(), "peer", routeParam(r, "id"), &p) != nil {
		http.Error(w, "host unavailable", 404)
		return
	}
	var out []Session
	if e := h.peerRequest(r.Context(), p, "/sessions", nil, &out); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	for i := range out {
		out[i].ID = p.ID + "::" + out[i].ID
		out[i].Name = p.Name + " / " + out[i].Name
	}
	writeJSON(w, 200, out)
}
func (h *APIHandler) peerTurn(ctx context.Context, peer, id, prompt string) (string, error) {
	var p Peer
	if e := h.store.Document(ctx, "peer", peer, &p); e != nil {
		return "", e
	}
	var out struct {
		Text string `json:"text"`
	}
	e := h.peerRequest(ctx, p, "/sessions/"+url.PathEscape(id)+"/turn", map[string]string{"prompt": prompt}, &out)
	return out.Text, e
}

type Transfer struct {
	ID          string    `json:"id"`
	Peer        string    `json:"peer"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Bytes       int       `json:"bytes"`
	SHA256      string    `json:"sha256"`
	Status      string    `json:"status"`
	At          time.Time `json:"at"`
}
type FilePayload struct {
	Name   string `json:"name"`
	Data   []byte `json:"data"`
	SHA256 string `json:"sha256"`
}

func (h *APIHandler) transfer(ctx context.Context, peer, path string) (string, error) {
	h.audit("you", "FILE_TRANSFER_REQUESTED", peer, map[string]any{"source": path})
	var p Peer
	if e := h.store.Document(ctx, "peer", peer, &p); e != nil {
		return "", e
	}
	root, e := os.OpenRoot(h.fileRoot)
	if e != nil {
		return "", e
	}
	defer root.Close()
	f, e := root.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if e != nil {
		return "", e
	}
	if len(data) > 32<<20 {
		return "", fmt.Errorf("prototype transfers support files up to 32 MiB")
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	var out struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	if e := h.peerRequest(ctx, p, "/transfers/receive", FilePayload{Name: filepath.Base(path), Data: data, SHA256: digest}, &out); e != nil {
		return "", e
	}
	if out.SHA256 != digest {
		return "", fmt.Errorf("transfer checksum mismatch")
	}
	t := Transfer{ID: newID(), Peer: peer, Source: path, Destination: out.Path, Bytes: len(data), SHA256: digest, Status: "completed", At: time.Now().UTC()}
	if e := h.store.PutDocument(ctx, "transfer", t.ID, t); e != nil {
		return "", e
	}
	h.audit("you", "FILE_TRANSFERRED", t.ID, map[string]any{"peer": peer, "bytes": len(data), "sha256": digest})
	return out.Path, nil
}
func (h *APIHandler) transfers(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		out, e := h.store.Documents(r.Context(), "transfer")
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var req struct {
		Peer string `json:"peer"`
		Path string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req) != nil {
		http.Error(w, "invalid transfer", 400)
		return
	}
	path, e := h.transfer(r.Context(), req.Peer, req.Path)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	writeJSON(w, 201, map[string]string{"path": path})
}
func (h *APIHandler) receive(w http.ResponseWriter, r *http.Request) {
	var payload FilePayload
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 45<<20)).Decode(&payload) != nil || len(payload.Data) > 32<<20 || payload.Name == "" {
		http.Error(w, "invalid file", 400)
		return
	}
	sum := sha256.Sum256(payload.Data)
	digest := hex.EncodeToString(sum[:])
	if digest != payload.SHA256 {
		http.Error(w, "checksum mismatch", 400)
		return
	}
	dir := ".mace-inbox"
	if h.fileRoot == "/" {
		home, _ := os.UserHomeDir()
		dir = strings.TrimPrefix(filepath.Join(home, ".local/share/mace/inbox"), "/")
	}
	root, e := os.OpenRoot(h.fileRoot)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	defer root.Close()
	if e := root.MkdirAll(dir, 0700); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	name := filepath.Base(payload.Name)
	if name == "." || name == "/" {
		http.Error(w, "invalid name", 400)
		return
	}
	path := filepath.Join(dir, newID()+"-"+name)
	f, e := root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	_, e = f.Write(payload.Data)
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		http.Error(w, "failed to save transfer", 500)
		return
	}
	h.audit("peer", "FILE_RECEIVED", path, map[string]any{"bytes": len(payload.Data), "sha256": digest})
	t := Transfer{ID: newID(), Peer: "incoming", Source: payload.Name, Destination: filepath.Join(h.fileRoot, path), Bytes: len(payload.Data), SHA256: digest, Status: "received", At: time.Now().UTC()}
	if e := h.store.PutDocument(r.Context(), "transfer", t.ID, t); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	writeJSON(w, 201, map[string]string{"path": filepath.Join(h.fileRoot, path), "sha256": digest})
}
