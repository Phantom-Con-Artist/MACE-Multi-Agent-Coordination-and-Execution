package api

import (
	"context"
	"encoding/json"
	"mace/pkg/protocol"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatDeliveryAndFiles(t *testing.T) {
	s, supervisor, store := setupTestServer(t, "secret")
	defer store.Close()
	defer supervisor.Close()
	root := t.TempDir()
	s.handler.fileRoot = root
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("hello media"), 0600); err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer secret")
		w := httptest.NewRecorder()
		s.Router().ServeHTTP(w, r)
		return w
	}
	a, err := supervisor.StartAgent(context.Background(), protocol.AgentLaunchRequest{Command: "cat", Name: "echo"})
	if err != nil {
		t.Fatal(err)
	}
	w := request("POST", "/api/v1/rooms", `{"name":"team","members":["`+a.ID+`"]}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var room Room
	json.Unmarshal(w.Body.Bytes(), &room)
	w = request("POST", "/api/v1/chats/"+room.ID+"/messages", `{"text":"hello","file":"note.txt"}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var m Message
	json.Unmarshal(w.Body.Bytes(), &m)
	if m.Delivery[a.ID] != "delivered" {
		t.Fatal(m)
	}
	w = request("GET", "/api/v1/chats/"+room.ID+"/messages", "")
	var history []Message
	json.Unmarshal(w.Body.Bytes(), &history)
	if len(history) != 1 || history[0].Text != "hello" {
		t.Fatal(w.Body.String())
	}
	w = request("POST", "/api/v1/chats/"+room.ID+"/messages", `{"sender":"stranger","text":"x"}`)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
	w = request("GET", "/api/v1/files?path=note.txt", "")
	if w.Code != 200 || w.Body.String() != "hello media" {
		t.Fatal(w.Code, w.Body.String())
	}
	w = request("GET", "/api/v1/files?path=../note.txt", "")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	if err := os.Symlink(filepath.Join(root, ".."), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	w = request("GET", "/api/v1/files?path=escape", "")
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
}
func TestBrowserSession(t *testing.T) {
	s, sup, store := setupTestServer(t, "secret")
	defer store.Close()
	defer sup.Close()
	r := httptest.NewRequest("POST", "/api/session", strings.NewReader(`{"token":"secret"}`))
	w := httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("missing secure session attributes")
	}
	r = httptest.NewRequest("GET", "/api/v1/node", nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	r = httptest.NewRequest("GET", "/api/v1/node", nil)
	w = httptest.NewRecorder()
	s.Router().ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
}
