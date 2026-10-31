package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"mace/internal/storage"
)

const deviceLifetime = 30 * 24 * time.Hour

// Device is one signed-in browser. Only a hash of its session secret is stored,
// and it is bound to the access token that created it, so rotating the token
// signs every device out.
type Device struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Hash      string    `json:"hash,omitempty"`
	TokenHash string    `json:"token_hash,omitempty"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"last_seen"`
	Revoked   bool      `json:"revoked,omitempty"`
	Current   bool      `json:"current,omitempty"`
}

type devices struct {
	store  *storage.Store
	secret string
	mu     sync.Mutex
}

func hashHex(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }

// legacyCookie is the value issued before per-device sessions existed.
func (d *devices) legacyCookie() string { return hashHex(d.secret) }

func deviceName(ua string) string {
	browser := "Browser"
	for _, b := range []string{"Edg", "Firefox", "SamsungBrowser", "Chrome", "Safari"} {
		if strings.Contains(ua, b+"/") {
			browser = map[string]string{"Edg": "Edge", "SamsungBrowser": "Samsung Internet"}[b]
			if browser == "" {
				browser = b
			}
			break
		}
	}
	system := ""
	for _, s := range []string{"Android", "iPhone", "iPad", "Windows", "Mac OS", "Linux"} {
		if strings.Contains(ua, s) {
			system = strings.Replace(s, "Mac OS", "macOS", 1)
			break
		}
	}
	if system == "" {
		return browser
	}
	return browser + " on " + system
}

func (d *devices) issue(w http.ResponseWriter, r *http.Request) error {
	secret := newID() + newID()
	now := time.Now().UTC()
	dev := Device{ID: "dev_" + newID()[:16], Name: deviceName(r.UserAgent()), Hash: hashHex(secret), TokenHash: hashHex(d.secret)[:16], Created: now, LastSeen: now}
	if err := d.store.PutDocument(r.Context(), "device", dev.ID, dev); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "mace_session", Value: dev.ID + "." + secret, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: int(deviceLifetime.Seconds())})
	return nil
}

// check validates the session cookie. It returns the device ID, whether the
// request is authorized, and whether it used a cookie from before devices existed.
func (d *devices) check(r *http.Request) (string, bool, bool) {
	cookie, err := r.Cookie("mace_session")
	if err != nil {
		return "", false, false
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(d.legacyCookie())) == 1 {
		return "", true, true
	}
	if d.store == nil {
		return "", false, false
	}
	id, secret, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return "", false, false
	}
	var dev Device
	if d.store.Document(r.Context(), "device", id, &dev) != nil || dev.Revoked {
		return "", false, false
	}
	if subtle.ConstantTimeCompare([]byte(hashHex(secret)), []byte(dev.Hash)) != 1 || dev.TokenHash != hashHex(d.secret)[:16] {
		return "", false, false
	}
	now := time.Now().UTC()
	if now.Sub(dev.LastSeen) > deviceLifetime {
		return "", false, false
	}
	if now.Sub(dev.LastSeen) > time.Minute {
		d.mu.Lock()
		dev.LastSeen = now
		d.store.PutDocument(context.Background(), "device", dev.ID, dev)
		d.mu.Unlock()
	}
	return dev.ID, true, false
}

func (d *devices) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health and public endpoints are exempt from auth.
		if r.URL.Path == "/health" || r.URL.Path == "/api/session" || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		// Joining is authorized by its one-time code, a node's identity is
		// public, and requests between nodes are checked by their handlers
		// against the sender's node key.
		if r.URL.Path == "/api/v1/nodes/accept" || r.URL.Path == "/api/v1/node/identity" ||
			(strings.HasPrefix(r.URL.Path, "/api/v1/node/") && r.Header.Get("X-Mace-Node") != "") {
			next.ServeHTTP(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.secret)) == 1 {
			next.ServeHTTP(w, r)
			return
		}
		_, ok, legacy := d.check(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if legacy && d.store != nil {
			// Upgrade silently so signed-in phones keep working.
			d.issue(w, r)
		}
		next.ServeHTTP(w, r)
	})
}

func (d *devices) list(w http.ResponseWriter, r *http.Request) {
	current, _, _ := d.check(r)
	raw, err := d.store.Documents(r.Context(), "device")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	out := []Device{}
	for _, b := range raw {
		var dev Device
		if json.Unmarshal(b, &dev) != nil || dev.Revoked || dev.TokenHash != hashHex(d.secret)[:16] || time.Since(dev.LastSeen) > deviceLifetime {
			continue
		}
		dev.Hash, dev.TokenHash, dev.Current = "", "", dev.ID == current
		out = append(out, dev)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen.After(out[j].LastSeen) })
	writeJSON(w, 200, out)
}

func (d *devices) revoke(w http.ResponseWriter, r *http.Request) {
	id := routeParam(r, "id")
	var dev Device
	if d.store.Document(r.Context(), "device", id, &dev) != nil {
		http.Error(w, "Device not found.", 404)
		return
	}
	dev.Revoked = true
	if err := d.store.PutDocument(r.Context(), "device", id, dev); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	d.store.AppendAudit(r.Context(), "you", "DEVICE_REVOKED", id, map[string]any{"name": dev.Name})
	writeJSON(w, 200, map[string]bool{"revoked": true})
}

// signOut revokes the calling device and clears its cookie.
func (d *devices) signOut(w http.ResponseWriter, r *http.Request) {
	if id, ok, _ := d.check(r); ok && id != "" {
		var dev Device
		if d.store.Document(r.Context(), "device", id, &dev) == nil {
			dev.Revoked = true
			d.store.PutDocument(r.Context(), "device", id, dev)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "mace_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"connected": false})
}
