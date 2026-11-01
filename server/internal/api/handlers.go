package api

import (
	"context"
	"encoding/json"
	"mace/internal/agent"
	"mace/internal/blobs"
	"mace/internal/records"
	"mace/internal/storage"
	"mace/pkg/protocol"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

type APIHandler struct {
	supervisor  *agent.Supervisor
	store       *storage.Store
	nodeInfo    protocol.NodeInfo
	startTime   time.Time
	fileRoot    string
	lifecycleMu sync.Mutex
	roomMu      sync.Mutex
	presence    presence
	sendMu      sync.Mutex
	events      *hub
	records     *records.Log
	// advertise overrides the address other machines use to reach this one.
	advertise  string
	port       int
	nodeHealth nodeState
	// sched guards the per-agent inboxes and per-room run state.
	sched     sync.Mutex
	runSaveMu sync.Mutex
	queues    map[string]*memberQueue
	roomRuns  map[string]*roomRun
	// sessionDocMu serializes read-modify-write of session documents.
	sessionDocMu sync.Mutex
	// statusMu guards status documents; statusActMu runs one offer, pick,
	// or close at a time on a status's home machine.
	// dataDir holds the session databases; restart moves the daemon into a
	// newly chosen session (see workspaces.go).
	dataDir     string
	restart     func()
	statusMu    sync.Mutex
	statusActMu sync.Mutex
	// turnLocks lets one native turn run per agent; local inbox turns and
	// turns requested by connected hosts wait for each other.
	turnLocks sync.Map
	machine   monitor
	// ctx serves mace ctx fetches; graphs keeps code graphs fresh.
	ctx    *contextService
	graphs codeGraphs
	// blobs holds chunks of files moving between machines.
	blobs     *blobs.Store
	blobsErr  error
	blobsOnce sync.Once
	push      pushState
	// bindHost is the address the server listens on, set when it starts.
	bindHost string
	// tail caches this device's view of the owner's tailnet.
	tail tailnetState
	// remoteRuns holds turns other machines asked for, by key.
	remoteRuns remoteRuns
}

func NewAPIHandler(supervisor *agent.Supervisor, store *storage.Store, nodeInfo protocol.NodeInfo) *APIHandler {
	if nodeInfo.StartedAt.IsZero() {
		nodeInfo.StartedAt = time.Now().UTC()
	}
	if nodeInfo.OS == "" {
		nodeInfo.OS = runtime.GOOS
	}
	if nodeInfo.Arch == "" {
		nodeInfo.Arch = runtime.GOARCH
	}
	if nodeInfo.HostName == "" {
		nodeInfo.HostName, _ = os.Hostname()
	}
	if store != nil {
		raw, _ := store.Documents(context.Background(), "run")
		for _, b := range raw {
			var run Run
			if json.Unmarshal(b, &run) == nil && run.Status == "running" {
				run.Status = "interrupted"
				run.ActiveMember, run.ActiveMembers = "", nil
				run.Error = "The host restarted while agents were working. Continue when you are ready."
				store.PutDocument(context.Background(), "run", run.ID, run)
			}
		}
		raw, _ = store.Documents(context.Background(), "session")
		for _, b := range raw {
			var s Session
			if json.Unmarshal(b, &s) == nil && s.Status == "replying" {
				s.Status = "ready"
				store.PutDocument(context.Background(), "session", s.ID, s)
			}
		}
	}
	h := &APIHandler{
		supervisor: supervisor,
		store:      store,
		nodeInfo:   nodeInfo,
		startTime:  time.Now(),
		presence:   presence{lastSeen: map[string]time.Time{}},
		events:     newHub(),
		queues:     map[string]*memberQueue{},
		roomRuns:   map[string]*roomRun{},
		ctx:        newContextService(),
	}
	if store != nil {
		h.ensureHandles(context.Background())
		h.records = openRecords(store)
	}
	return h
}

func (h *APIHandler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"application":    "mace",
		"version":        h.nodeInfo.Version,
		"status":         "ok",
		"uptime_seconds": uint64(time.Since(h.startTime).Seconds()),
		"node_id":        h.nodeInfo.ID,
		"role":           h.nodeInfo.Role,
		"api":            apiVersion,
	})
}

func (h *APIHandler) GetNodeInfo(w http.ResponseWriter, r *http.Request) {
	agents, _ := h.supervisor.ListAgents(r.Context())
	info := h.nodeInfo
	info.ActiveAgents = 0
	for _, a := range agents {
		if !a.Status.IsTerminal() {
			info.ActiveAgents++
		}
	}
	info.UptimeSeconds = uint64(time.Since(h.startTime).Seconds())
	writeJSON(w, http.StatusOK, info)
}

func (h *APIHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := h.supervisor.ListAgents(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []*protocol.AgentProcess{}
	}
	writeJSON(w, http.StatusOK, agents)
}

func (h *APIHandler) LaunchAgent(w http.ResponseWriter, r *http.Request) {
	var req protocol.AgentLaunchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	proc, err := h.supervisor.StartAgent(r.Context(), req)
	if err != nil {
		if err == agent.ErrPoolCapacityFull {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	writeJSON(w, http.StatusCreated, proc)
}

func (h *APIHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	agentID := routeParam(r, "id")
	proc, err := h.supervisor.GetAgent(agentID)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	if proc == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, proc)
}

func (h *APIHandler) GetAgentEvents(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		http.Error(w, "event storage unavailable", http.StatusNotImplemented)
		return
	}

	agentID := routeParam(r, "id")
	afterSeq, _ := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 100
	}

	events, err := h.store.GetAgentEvents(r.Context(), agentID, afterSeq, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []*protocol.AgentEvent{}
	}
	writeJSON(w, http.StatusOK, events)
}

func (h *APIHandler) SendAgentInput(w http.ResponseWriter, r *http.Request) {
	agentID := routeParam(r, "id")
	var req protocol.AgentInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.supervisor.SendInput(agentID, req.Data); err != nil {
		if err == agent.ErrAgentNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err == agent.ErrAgentNotRunning {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "delivered"})
}

func (h *APIHandler) StopAgent(w http.ResponseWriter, r *http.Request) {
	agentID := routeParam(r, "id")
	var req protocol.AgentStopRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // Optional body

	timeout := time.Duration(req.GraceTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 2 * time.Second
	}

	if err := h.supervisor.StopAgent(agentID, req.Signal, timeout); err != nil {
		if err == agent.ErrAgentNotFound {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
