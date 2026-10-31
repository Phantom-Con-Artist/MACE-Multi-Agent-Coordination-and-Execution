package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"mace/internal/blobs"
)

// Files move between joined machines as content-addressed chunks: the sender
// stores the file as chunks and a manifest, then asks the receiver to fetch
// it. The receiver pulls only the chunks it lacks, verifies each against its
// hash, and assembles the file, so transfers resume after a dropped
// connection and have no size cap. Hosts connected with a token keep the
// older single-request copy.

// transferLimit bounds one chunked transfer.
const transferLimit = 30 * time.Minute

func (h *APIHandler) blobStore() (*blobs.Store, error) {
	h.blobsOnce.Do(func() {
		base := os.TempDir()
		if h.store != nil && h.store.Path() != ":memory:" {
			base = filepath.Dir(h.store.Path())
		} else {
			base = filepath.Join(base, "mace-blobs-"+newID())
		}
		h.blobs, h.blobsErr = blobs.Open(filepath.Join(base, "blobs"))
	})
	return h.blobs, h.blobsErr
}

// blob serves GET /api/v1/blobs/{hash}: a stored chunk or manifest.
func (h *APIHandler) blob(w http.ResponseWriter, r *http.Request) {
	store, err := h.blobStore()
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	b, err := store.Get(routeParam(r, "hash"))
	if err != nil {
		http.Error(w, "not stored here", 404)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(b)
}

// sendChunked offers a file under the file root to a joined node and returns
// where the node saved it.
func (h *APIHandler) sendChunked(ctx context.Context, p Peer, path string) (string, blobs.Manifest, error) {
	store, err := h.blobStore()
	if err != nil {
		return "", blobs.Manifest{}, err
	}
	root, err := os.OpenRoot(h.fileRoot)
	if err != nil {
		return "", blobs.Manifest{}, err
	}
	defer root.Close()
	f, err := root.Open(path)
	if err != nil {
		return "", blobs.Manifest{}, err
	}
	defer f.Close()
	m, err := store.Add(f, filepath.Base(path), "")
	if err != nil {
		return "", m, err
	}
	var out struct {
		Path string `json:"path"`
	}
	if err := h.peerRequest(ctx, p, "/transfers/fetch", map[string]string{"manifest": m.ID()}, &out); err != nil {
		return "", m, err
	}
	return out.Path, m, nil
}

// fetchTransfer serves POST /api/v1/transfers/fetch from a joined node: pull
// the named file from that node and save it in this machine's inbox.
func (h *APIHandler) fetchTransfer(w http.ResponseWriter, r *http.Request) {
	var from Node
	if h.store.Document(r.Context(), "node", r.Header.Get("X-Mace-Node"), &from) != nil {
		http.Error(w, "only a joined machine can offer a file", 403)
		return
	}
	var req struct {
		Manifest string `json:"manifest"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || !blobs.ValidHash(req.Manifest) {
		http.Error(w, "invalid transfer", 400)
		return
	}
	path, m, err := h.pullFile(r.Context(), from, req.Manifest)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	t := Transfer{ID: newID(), Peer: from.ID, Source: m.Name, Destination: path, Bytes: int(m.Size), SHA256: req.Manifest, Status: "received", At: time.Now().UTC()}
	h.store.PutDocument(r.Context(), "transfer", t.ID, t)
	h.audit("peer", "FILE_RECEIVED", path, map[string]any{"bytes": m.Size, "manifest": req.Manifest, "from": from.Name, "chunks": len(m.Chunks)})
	writeJSON(w, 201, map[string]string{"path": path, "manifest": req.Manifest})
}

// pullFile fetches a manifest and its missing chunks from a node, verifying
// each, and assembles the file in the inbox.
func (h *APIHandler) pullFile(ctx context.Context, from Node, id string) (string, blobs.Manifest, error) {
	store, err := h.blobStore()
	if err != nil {
		return "", blobs.Manifest{}, err
	}
	get := func(hash string) error {
		if store.Has(hash) {
			return nil
		}
		var buf bytes.Buffer
		for attempt := 0; ; attempt++ {
			buf.Reset()
			err := h.signedRequest(ctx, from.Name, from.URL, "GET", "/api/v1/blobs/"+url.PathEscape(hash), nil, &limitedWriter{w: &buf, n: blobs.ChunkSize + 1<<20}, time.Minute)
			if err == nil {
				return store.Put(hash, buf.Bytes())
			}
			if attempt == 2 || ctx.Err() != nil {
				return err
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	if err := get(id); err != nil {
		return "", blobs.Manifest{}, fmt.Errorf("could not get the file's manifest: %w", err)
	}
	m, err := store.Manifest(id)
	if err != nil {
		return "", m, err
	}
	for _, c := range store.Missing(m) {
		if err := get(c); err != nil {
			return "", m, fmt.Errorf("could not get part of %s: %w", m.Name, err)
		}
	}
	dir := h.inboxDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", m, err
	}
	name := filepath.Base(m.Name)
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	dest := filepath.Join(dir, newID()[:8]+"-"+name)
	return dest, m, store.Assemble(m, dest)
}

// inboxDir is where files from other machines are saved, inside the file root.
func (h *APIHandler) inboxDir() string {
	if h.fileRoot == "/" {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local/share/mace/inbox")
	}
	return filepath.Join(h.fileRoot, ".mace-inbox")
}

// limitedWriter refuses content beyond n bytes, so a misbehaving node cannot
// fill memory with one chunk.
type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.w.Len()+len(p) > l.n {
		return 0, errors.New("chunk larger than allowed")
	}
	return l.w.Write(p)
}
