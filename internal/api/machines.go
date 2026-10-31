package api

import (
	"bytes"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// The machines page browses every joined machine from the hub: its files
// (with previews streamed as they play), its desktop apps, and launching one.
// The hub forwards these requests to the machine, signed with its node key,
// and streams the answer back unchanged.

// machinePaths are the requests the hub forwards, by method.
var machinePaths = map[string]string{"files": "GET", "apps": "GET", "host/open": "POST", "catalog": "GET", "metrics": "GET"}

// forwardedHeaders pass through in each direction, so ranges, caching, and
// the sandboxing of previewed files survive the trip.
var (
	requestHeaders  = []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"}
	responseHeaders = []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "Last-Modified", "ETag", "X-Content-Type-Options", "Content-Security-Policy"}
)

// machineProxy serves /api/v1/machines/{id}/{path}.
func (h *APIHandler) machineProxy(w http.ResponseWriter, r *http.Request) {
	var p Peer
	if h.peerDoc(r.Context(), routeParam(r, "id"), &p) != nil {
		http.Error(w, "That machine is not joined here.", 404)
		return
	}
	rest := chi.URLParam(r, "*")
	if machinePaths[rest] != r.Method {
		http.Error(w, "not available on other machines", 404)
		return
	}
	var body []byte
	if r.Method == "POST" {
		var err error
		if body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 65536)); err != nil {
			http.Error(w, "request too large", 400)
			return
		}
	}
	target := p.URL + "/api/v1/" + rest
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	for _, k := range requestHeaders {
		if v := r.Header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	if p.Node != "" {
		h.sign(req, body)
	} else {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	client := &http.Client{Transport: peerTransport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		http.Error(w, p.Name+" did not answer: "+err.Error(), 502)
		return
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized && p.Node != "" {
		// A joined machine refuses a signed request only when its MACE is
		// older than this one and does not offer that yet.
		http.Error(w, p.Name+" runs an older MACE that does not offer this yet. Update it there: git pull && make install && systemctl --user restart mace", 502)
		return
	}
	for _, k := range responseHeaders {
		if v := res.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}
