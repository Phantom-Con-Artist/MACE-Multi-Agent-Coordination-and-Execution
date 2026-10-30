package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"mace/internal/agent"
	"mace/internal/api"
	"mace/internal/bootstrap"
	"mace/internal/concurrency"
	"mace/internal/network/tailscale"
	"mace/internal/storage"
	"mace/pkg/protocol"
)

var (
	version = bootstrap.Version
)

func main() {
	roleFlag := flag.String("role", getEnv("MACE_NODE_ROLE", "worker"), "Operational role: 'worker' or 'controller'")
	hostFlag := flag.String("host", getEnv("MACE_HTTP_HOST", "0.0.0.0"), "HTTP host to bind")
	portFlag := flag.Int("port", getEnvInt("MACE_HTTP_PORT", 8080), "HTTP port to bind")
	dbFlag := flag.String("db", getEnv("MACE_DB_PATH", filepath.Join(bootstrap.DataDir(), "mace.db")), "Path to SQLite database")
	maxAgentsFlag := flag.Int("max-agents", getEnvInt("MACE_MAX_CONCURRENT_AGENTS", 10), "Max concurrent running agents")
	authSecretFlag := flag.String("auth-secret", getEnv("MACE_AUTH_SECRET", ""), "API Bearer token secret (optional)")

	// Tailscale flags
	tsAuthKeyFlag := flag.String("ts-authkey", getEnv("MACE_TS_AUTHKEY", ""), "Tailscale Auth Key (optional, runs embedded tsnet)")
	tsHostnameFlag := flag.String("ts-hostname", getEnv("MACE_TS_HOSTNAME", "mace-node"), "Tailscale node hostname")
	tsStateDirFlag := flag.String("ts-statedir", getEnv("MACE_TS_STATE_DIR", "./data/tailscale"), "Tailscale state directory")
	tsEphemeralFlag := flag.Bool("ts-ephemeral", getEnvBool("MACE_TS_EPHEMERAL", true), "Register as ephemeral Tailscale node")

	flag.Parse()
	if *authSecretFlag == "" {
		token, e := bootstrap.Token(filepath.Join(filepath.Dir(*dbFlag), "access-token"))
		if e != nil {
			log.Fatal(e)
		}
		*authSecretFlag = token
	}

	log.Printf("[MACE] Starting MACE daemon v%s (Role: %s)", version, *roleFlag)

	// Ensure data directory exists
	dataDir := filepath.Dir(*dbFlag)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("[MACE] Failed to create data directory: %v", err)
	}

	// 1. Initialize SQLite Store
	store, err := storage.Open(*dbFlag)
	if err != nil {
		log.Fatalf("[MACE] Failed to open database: %v", err)
	}
	defer store.Close()
	log.Printf("[MACE] Database initialized at %s", *dbFlag)

	// 2. Initialize Agent Supervisor
	agentSupervisor := agent.NewSupervisor(store, *maxAgentsFlag)
	defer agentSupervisor.Close()
	log.Printf("[MACE] Supervisor ready (Max concurrent agents: %d)", *maxAgentsFlag)

	// 3. Initialize Networking (Tailscale or local TCP)
	tsNode, err := tailscale.NewNode(tailscale.Config{
		Host:      *hostFlag,
		AuthKey:   *tsAuthKeyFlag,
		Hostname:  *tsHostnameFlag,
		StateDir:  *tsStateDirFlag,
		Ephemeral: *tsEphemeralFlag,
		Port:      *portFlag,
	})
	if err != nil {
		log.Fatalf("[MACE] Failed to configure network node: %v", err)
	}
	defer tsNode.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listener, err := tsNode.Listen(ctx)
	if err != nil {
		log.Fatalf("[MACE] Failed to bind network listener: %v", err)
	}
	defer listener.Close()

	if tsNode.IsTailscaleEnabled() {
		log.Printf("[MACE] Network: Embedded Tailscale active on %s (Tailnet Host: %s:%d)", listener.Addr(), *tsHostnameFlag, *portFlag)
	} else {
		log.Printf("[MACE] Network: Local TCP listener bound to http://%s", listener.Addr())
	}

	// 4. Build API Server
	nodeID := fmt.Sprintf("%s-%d", *roleFlag, os.Getpid())
	nodeInfo := protocol.NodeInfo{
		ID:        nodeID,
		Name:      *tsHostnameFlag,
		Role:      protocol.NodeRole(*roleFlag),
		Version:   version,
		StartedAt: time.Now().UTC(),
	}

	apiServer := api.NewServer(api.Config{
		Host:       *hostFlag,
		Port:       *portFlag,
		AuthSecret: *authSecretFlag,
		NodeInfo:   nodeInfo,
		Listener:   listener,
	}, agentSupervisor, store)

	// 5. Manage Services with Supervisor Group
	group := concurrency.NewSupervisorGroup()
	group.Add(apiServer)

	if err := group.Start(ctx); err != nil {
		log.Fatalf("[MACE] Failed to start services: %v", err)
	}

	log.Printf("[MACE] Daemon ready. Press Ctrl+C to terminate.")
	if path := os.Getenv("MACE_READY_FILE"); path != "" {
		if err := os.WriteFile(path, []byte(listener.Addr().String()), 0600); err != nil {
			log.Printf("[MACE] Could not write readiness record: %v", err)
		}
	}

	// 6. Handle Shutdown Signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("[MACE] Caught signal %v, initiating graceful shutdown...", sig)
	case err := <-group.Errors():
		log.Printf("[MACE] Unexpected service error: %v, shutting down...", err)
	}

	cancel()
	group.Stop(5 * time.Second)
	_ = agentSupervisor.Close()
	log.Printf("[MACE] Daemon terminated cleanly.")
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		return val == "true" || val == "1" || val == "yes"
	}
	return fallback
}
