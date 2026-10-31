package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"mace/internal/qrcode"
)

// Sign-in links in QR codes and links handed from one machine to another
// carry a one-time code instead of the access token. A code works once,
// within a few minutes, and signs in one device.

func (h *APIHandler) newSignInCode(ctx context.Context, ttl time.Duration) (string, error) {
	b := make([]byte, 18)
	rand.Read(b)
	code := hex.EncodeToString(b)
	return code, h.store.PutDocument(ctx, "signin-code", hashHex(code), map[string]time.Time{"expires": time.Now().Add(ttl)})
}

// spendSignInCode reports whether a code was valid, using it up.
func (h *APIHandler) spendSignInCode(ctx context.Context, code string) bool {
	var c map[string]time.Time
	key := hashHex(strings.TrimSpace(code))
	if h.store.Document(ctx, "signin-code", key, &c) != nil {
		return false
	}
	spent, err := h.store.DeleteDocument(ctx, "signin-code", key)
	return err == nil && spent && time.Now().Before(c["expires"])
}

// publicURL is the address a phone should open: the advertised or HTTPS
// address if there is one, else the address this request came in on.
func (h *APIHandler) publicURL(r *http.Request) string {
	explicit := h.advertise != "" || os.Getenv("MACE_PUBLIC_URL") != ""
	if ip := net.ParseIP(h.bindHost); explicit || ip == nil || !ip.IsLoopback() {
		// A server listening only on this computer is reachable only where
		// the page was opened, unless it is published, as mace-server does.
		if u := h.selfURL(); u != "" {
			return u
		}
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// signInQR serves GET /api/v1/signin/qr: a one-time sign-in link for another
// device and its QR code.
func (h *APIHandler) signInQR(w http.ResponseWriter, r *http.Request) {
	code, err := h.newSignInCode(r.Context(), 10*time.Minute)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	link := h.publicURL(r) + "/#connect-once=" + code
	svg, err := qrcode.SVG(link)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]any{"link": link, "svg": svg, "expires_in": 600})
}

// nodeSignIn serves POST /api/v1/node/signin from a joined machine: a
// one-time link that signs the owner's device in here.
func (h *APIHandler) nodeSignIn(w http.ResponseWriter, r *http.Request) {
	if _, _, err := h.verifyNodeRequest(r); err != nil {
		http.Error(w, err.Error(), 401)
		return
	}
	code, err := h.newSignInCode(r.Context(), 2*time.Minute)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, 200, map[string]string{"url": h.selfURL() + "/#connect-once=" + code})
}

// openNode serves POST /api/v1/nodes/{id}/open: a signed-in link to another
// of the owner's machines, so a phone can switch between them.
func (h *APIHandler) openNode(w http.ResponseWriter, r *http.Request) {
	var n Node
	if h.store.Document(r.Context(), "node", routeParam(r, "id"), &n) != nil {
		http.Error(w, "That machine is not joined here.", 404)
		return
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := h.nodeRequest(r.Context(), n, "POST", "/api/v1/node/signin", map[string]any{}, &out); err != nil {
		http.Error(w, n.Name+" did not answer: "+err.Error(), 502)
		return
	}
	writeJSON(w, 200, out)
}
