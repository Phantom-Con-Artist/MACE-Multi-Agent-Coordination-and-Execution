package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mace/internal/agent"
	"mace/pkg/protocol"
)

func request(s *Server, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 15) Chrome/140.0 Mobile Safari/537.36")
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	return w
}

func sessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == "mace_session" && c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie issued")
	return nil
}

func TestDeviceSessions(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()

	if w := request(s, "POST", "/api/session", `{"token":"wrong"}`); w.Code != 401 {
		t.Fatalf("wrong token accepted: %d", w.Code)
	}
	phone := sessionCookie(t, request(s, "POST", "/api/session", `{"token":"secret"}`))
	laptop := sessionCookie(t, request(s, "POST", "/api/session", `{"token":"secret"}`))
	// Signing in again from an already signed-in browser keeps its device.
	if w := request(s, "POST", "/api/session", `{"token":"secret"}`, phone); len(w.Result().Cookies()) != 0 {
		t.Fatal("re-opening the sign-in link created another device")
	}
	if phone.Value == laptop.Value || strings.Contains(phone.Value, "secret") {
		t.Fatal("each device needs its own opaque session")
	}
	if w := request(s, "GET", "/api/v1/devices", "", phone); w.Code != 200 || !strings.Contains(w.Body.String(), "Chrome on Android") || strings.Contains(w.Body.String(), `"hash"`) {
		t.Fatalf("device list wrong: %d %s", w.Code, w.Body.String())
	}

	id, _, _ := strings.Cut(phone.Value, ".")
	if w := request(s, "POST", "/api/v1/devices/"+id+"/revoke", "", laptop); w.Code != 200 {
		t.Fatalf("revoke failed: %d", w.Code)
	}
	if w := request(s, "GET", "/api/v1/devices", "", phone); w.Code != 401 {
		t.Fatal("revoked device still authorized")
	}
	if w := request(s, "GET", "/api/v1/devices", "", laptop); w.Code != 200 {
		t.Fatal("revoking one device must not sign out the others")
	}

	forged := &http.Cookie{Name: "mace_session", Value: strings.SplitN(laptop.Value, ".", 2)[0] + ".guess"}
	if w := request(s, "GET", "/api/v1/devices", "", forged); w.Code != 401 {
		t.Fatal("forged cookie accepted")
	}

	// A cookie from before device sessions keeps working and is upgraded.
	legacy := &http.Cookie{Name: "mace_session", Value: hashHex("secret")}
	w := request(s, "GET", "/api/v1/devices", "", legacy)
	if w.Code != 200 {
		t.Fatal("legacy cookie rejected")
	}
	upgraded := sessionCookie(t, w)
	if upgraded.Value == legacy.Value {
		t.Fatal("legacy cookie was not upgraded")
	}

	// Rotating the access token signs every device out.
	rotated := NewServer(Config{Host: "127.0.0.1", AuthSecret: "new-secret", NodeInfo: protocol.NodeInfo{ID: "n"}}, agent.NewSupervisor(store, 1), store)
	if w := request(rotated, "GET", "/api/v1/devices", "", laptop); w.Code != 401 {
		t.Fatal("device session survived token rotation")
	}

	w = request(s, "POST", "/api/session/sign-out", "", laptop)
	if w := request(s, "GET", "/api/v1/devices", "", laptop); w.Code != 401 {
		t.Fatal("signed-out device still authorized")
	}
}
