package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
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
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(corsMiddleware)

	if cfg.AuthSecret != "" {
		r.Use(authMiddleware(cfg.AuthSecret))
	}

	handler := NewAPIHandler(supervisor, store, cfg.NodeInfo)

	// Route bindings
	r.Get("/health", handler.Health)

	r.Route("/api/v1", func(r chi.Router) {
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

func authMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Health and public endpoints exempt from auth
			if r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			expected := "Bearer " + secret
			if authHeader != expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
