package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"mace/internal/agent"
	"mace/internal/storage"
	"mace/pkg/protocol"
)

// Server coordinates the HTTP REST & WebSocket control plane.
type Server struct {
	router     *chi.Mux
	httpServer *http.Server
	listener   net.Listener
	handler    *APIHandler
	addr       string
}

// Config specifies server configuration parameters.
type Config struct {
	Host       string
	Port       int
	AuthSecret string
	NodeInfo   protocol.NodeInfo
	Listener   net.Listener // Optional custom listener (e.g. tsnet)
}

// NewServer builds and mounts all REST and WebSocket routes on chi router.
func NewServer(cfg Config, supervisor *agent.Supervisor, store *storage.Store) *Server {
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	if cfg.Port == 0 && cfg.Listener == nil {
		addr = "127.0.0.1:8080"
	}

	r := chi.NewRouter()

	// Standard middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Long-lived routes manage their own limits.
			for _, suffix := range []string{"/turn", "/ws", "/stream", "/transfers/receive"} {
				if strings.HasSuffix(r.URL.Path, suffix) {
					next.ServeHTTP(w, r)
					return
				}
			}
			middleware.Timeout(60*time.Second)(next).ServeHTTP(w, r)
		})
	})
	r.Use(corsMiddleware)

	handler := NewAPIHandler(supervisor, store, cfg.NodeInfo)
	handler.fileRoot = os.Getenv("MACE_FILE_ROOT")
	if handler.fileRoot == "" {
		handler.fileRoot = "/"
	}
	signIns := &devices{store: store, secret: cfg.AuthSecret}
	if cfg.AuthSecret != "" {
		r.Use(signIns.middleware)
	}
	r.Post("/api/session", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || subtle.ConstantTimeCompare([]byte(body.Token), []byte(cfg.AuthSecret)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if store == nil {
			http.Error(w, "storage unavailable", 503)
			return
		}
		// Opening the sign-in link again keeps this browser's existing device.
		if id, ok, _ := signIns.check(r); ok && id != "" {
			writeJSON(w, 200, map[string]bool{"connected": true})
			return
		}
		if e := signIns.issue(w, r); e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		writeJSON(w, 200, map[string]bool{"connected": true})
	})
	r.Get("/api/session", func(w http.ResponseWriter, r *http.Request) {
		_, ok, legacy := signIns.check(r)
		if legacy && store != nil {
			signIns.issue(w, r)
		}
		writeJSON(w, 200, map[string]bool{"connected": cfg.AuthSecret == "" || ok})
	})
	r.Post("/api/session/sign-out", signIns.signOut)

	// Route bindings
	r.Get("/health", handler.Health)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/catalog", handler.catalog)
		r.Get("/devices", signIns.list)
		r.Post("/devices/{id}/revoke", signIns.revoke)
		r.Get("/sessions", handler.sessions)
		r.Post("/sessions", handler.sessions)
		r.Post("/sessions/{id}/turn", handler.remoteTurn)
		r.Post("/sessions/{id}/control", handler.controlSession)
		r.Post("/sessions/{id}/profile", handler.profile)
		r.Post("/sessions/{id}/command", handler.command)
		r.Get("/sessions/{id}/models", handler.models)
		r.Get("/avatars/{provider}", handler.avatar)
		r.Post("/rooms/{room}/members/{member}/remove", handler.removeMember)
		r.Post("/rooms/{room}/remove", handler.removeGroup)
		r.Delete("/rooms/{room}", handler.removeGroup)
		r.Post("/rooms/{room}/members/{member}/role", handler.memberRole)
		r.Post("/rooms/{room}/members", handler.addMember)
		r.Get("/apps", handler.apps)
		r.Post("/host/open", handler.hostAction)
		r.Get("/activity", handler.activity)
		r.Get("/runs", handler.runsList)
		r.Get("/chats/{room}/policy", handler.policy)
		r.Post("/chats/{room}/policy", handler.policy)
		r.Post("/chats/{room}/pause", handler.pause)
		r.Post("/chats/{room}/start", handler.startDiscussion)
		r.Post("/chats/{room}/continue", handler.continueConversation)
		r.Post("/pause-all", handler.pauseAll)
		r.Get("/stream", handler.stream)
		r.Get("/metrics", handler.metrics)
		r.Get("/peers/{id}/metrics", handler.peerMetrics)
		r.Post("/inbox", handler.inbox)
		r.Get("/chats/{room}/receipts", handler.receipts)
		r.Post("/presence", handler.presencePing)
		r.Post("/presence/leave", handler.presenceLeave)
		r.Get("/peers", handler.peers)
		r.Post("/peers", handler.peers)
		r.Get("/peers/{id}/sessions", handler.peerSessions)
		r.Get("/transfers", handler.transfers)
		r.Post("/transfers", handler.transfers)
		r.Post("/transfers/receive", handler.receive)
		r.Get("/files", handler.files)
		r.Get("/rooms", handler.rooms)
		r.Post("/rooms", handler.rooms)
		r.Get("/chats/{room}/messages", handler.messages)
		r.Post("/chats/{room}/messages", handler.messages)
		r.Get("/node", handler.GetNodeInfo)

		r.Route("/agents", func(r chi.Router) {
			r.Get("/", handler.ListAgents)
			r.Post("/", handler.LaunchAgent)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", handler.GetAgent)
				r.Get("/events", handler.GetAgentEvents)
				r.Post("/input", handler.SendAgentInput)
				r.Post("/stop", handler.StopAgent)
				r.Get("/ws", handleAgentWebSocket(supervisor))
			})
		})
	})
	webDir := os.Getenv("MACE_WEB_DIR")
	if webDir == "" {
		webDir = "web/dist"
	}
	r.Handle("/*", http.FileServer(http.Dir(webDir)))

	srv := &Server{
		router:   r,
		listener: cfg.Listener,
		handler:  handler,
		addr:     addr,
		httpServer: &http.Server{
			Addr:              addr,
			Handler:           r,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       120 * time.Second,
		},
	}

	return srv
}

// Name returns service identifier.
func (s *Server) Name() string {
	return "api-server"
}

// Router exposes the underlying chi.Mux (useful for testing).
func (s *Server) Router() *chi.Mux {
	return s.router
}

// Start listens and serves incoming HTTP requests until context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	var errChan = make(chan error, 1)
	go s.handler.guardPresence(ctx)
	go s.handler.watchMachine(ctx)

	go func() {
		if s.listener != nil {
			errChan <- s.httpServer.Serve(s.listener)
		} else {
			errChan <- s.httpServer.ListenAndServe()
		}
	}()

	select {
	case <-ctx.Done():
		return s.Stop(context.Background())
	case err := <-errChan:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Stop initiates a graceful shutdown of the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	s.handler.stopAllRuns(errShutdown)
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(shutdownCtx)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
