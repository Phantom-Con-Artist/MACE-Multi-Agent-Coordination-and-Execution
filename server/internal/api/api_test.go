package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"mace/internal/agent"
	"mace/internal/storage"
	"mace/pkg/protocol"
)

func setupTestServer(t *testing.T, authSecret string) (*Server, *agent.Supervisor, *storage.Store) {
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}

	supervisor := agent.NewSupervisor(store, 5)
	cfg := Config{
		Host:       "127.0.0.1",
		Port:       0,
		AuthSecret: authSecret,
		NodeInfo: protocol.NodeInfo{
			ID:   "test-node-1",
			Name: "test-node",
			Role: protocol.NodeRoleWorker,
		},
	}

	server := NewServer(cfg, supervisor, store)
	// Tests never consult the real tailnet of the machine running them.
	server.handler.tail.source = func() (tailnetView, error) { return tailnetView{}, errors.New("no tailnet in tests") }
	return server, supervisor, store
}

func TestAPI_HealthAndNodeInfo(t *testing.T) {
	server, supervisor, store := setupTestServer(t, "")
	defer supervisor.Close()
	defer store.Close()

	// 1. /health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /health, got %d", rec.Code)
	}

	var healthResp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if healthResp["status"] != "ok" || healthResp["role"] != "worker" {
		t.Errorf("unexpected health payload: %+v", healthResp)
	}

	// 2. /api/v1/node
	req = httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	rec = httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/node, got %d", rec.Code)
	}

	var nodeInfo protocol.NodeInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &nodeInfo); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if nodeInfo.ID != "test-node-1" {
		t.Errorf("expected node ID test-node-1, got %s", nodeInfo.ID)
	}
}

func TestAPI_AgentEndpoints(t *testing.T) {
	server, supervisor, store := setupTestServer(t, "")
	defer supervisor.Close()
	defer store.Close()

	// 1. Launch Agent
	launchBody := protocol.AgentLaunchRequest{
		Name:    "api-runner",
		Command: "bash",
		Args:    []string{"-c", "echo 'api output line'"},
	}
	bodyBytes, _ := json.Marshal(launchBody)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", bytes.NewReader(bodyBytes))
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var proc protocol.AgentProcess
	if err := json.Unmarshal(rec.Body.Bytes(), &proc); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if proc.ID == "" || proc.Name != "api-runner" {
		t.Errorf("invalid agent process returned: %+v", proc)
	}

	// 2. Get Agent
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+proc.ID, nil)
	rec = httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for get agent, got %d", rec.Code)
	}

	// 3. List Agents
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rec = httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for list agents, got %d", rec.Code)
	}
	var list []*protocol.AgentProcess
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 agent in list, got %d", len(list))
	}
}

func TestAPI_AuthMiddleware(t *testing.T) {
	server, supervisor, store := setupTestServer(t, "secret-token-123")
	defer supervisor.Close()
	defer store.Close()

	// Request without auth header should fail
	req := httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	rec := httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
	}

	// Request with valid auth header should succeed
	req = httptest.NewRequest(http.MethodGet, "/api/v1/node", nil)
	req.Header.Set("Authorization", "Bearer secret-token-123")
	rec = httptest.NewRecorder()
	server.Router().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with auth, got %d", rec.Code)
	}
}

func TestAPI_WebSocketStreaming(t *testing.T) {
	server, supervisor, store := setupTestServer(t, "")
	defer supervisor.Close()
	defer store.Close()

	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	// Launch cat agent for interactive streaming
	proc, err := supervisor.StartAgent(context.Background(), protocol.AgentLaunchRequest{
		Name:        "ws-cat",
		Command:     "cat",
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("failed to start agent: %v", err)
	}
	defer supervisor.StopAgent(proc.ID, "SIGKILL", 1*time.Second)

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/agents/" + proc.ID + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer ws.Close()

	// Send input through websocket
	sendMsg := wsClientMessage{
		Action: "input",
		Data:   "websocket test ping\n",
	}
	if err := ws.WriteJSON(sendMsg); err != nil {
		t.Fatalf("failed to send websocket input: %v", err)
	}

	// Expect echoed event
	var receivedEcho bool
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	for i := 0; i < 5; i++ {
		var event protocol.AgentEvent
		if err := ws.ReadJSON(&event); err != nil {
			break
		}
		if event.Type == protocol.EventStdout && strings.Contains(event.Data, "websocket test ping") {
			receivedEcho = true
			break
		}
	}

	if !receivedEcho {
		t.Fatal("did not receive echoed websocket output")
	}
}
