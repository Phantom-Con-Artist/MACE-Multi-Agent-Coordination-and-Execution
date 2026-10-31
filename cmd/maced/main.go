package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
	if path := os.Getenv("MACE_AUTH_SECRET_FILE"); *authSecretFlag == "" && path != "" {
		b, e := os.ReadFile(path)
		if e != nil {
			log.Fatalf("[MACE] Cannot read access token file: %v", e)
		}
		*authSecretFlag = strings.TrimSpace(string(b))
	}
	// Children are launched with a filtered environment, but nothing else in this
	// process should hand credentials on either.
	for _, key := range []string{"MACE_AUTH_SECRET", "MACE_AUTH_SECRET_FILE", "MACE_TS_AUTHKEY"} {
		os.Unsetenv(key)
	}
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
	if store.Recovered != "" {
		log.Printf("[MACE] WARNING: %s", store.Recovered)
		_ = store.AppendAudit(context.Background(), "storage", "DATABASE_RECOVERED", "", map[string]any{"details": store.Recovered})
	}

	// 2. Initialize Agent Supervisor
	agentSupervisor := agent.NewSupervisor(store, *maxAgentsFlag)
	defer agentSupervisor.Close()
	installation := installationID(*dbFlag)
	if n, e := agent.ReapOrphans(installation); e != nil {
		log.Printf("[MACE] Could not check for leftover agent processes: %v", e)
	} else if n > 0 {
		log.Printf("[MACE] Stopped %d agent processes left behind by an earlier daemon", n)
		_ = store.AppendAudit(context.Background(), "supervisor", "ORPHANS_STOPPED", "", map[string]any{"count": n})
	}
	agentSupervisor.SetProcessTag(agent.ProcessTag(installation, os.Getpid()))
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

	go maintain(ctx, store, time.Duration(getEnvInt("MACE_EVENT_RETENTION_DAYS", 14))*24*time.Hour)

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
	// A hangup arrives when the terminal or desktop session that ran mace
	// start closes. The server is meant to outlive it, and an unhandled SIGHUP
	// would end it silently, so it is logged and ignored.
	hupCh := make(chan os.Signal, 1)
	signal.Notify(hupCh, syscall.SIGHUP)

wait:
	for {
		select {
		case <-hupCh:
			log.Printf("[MACE] Ignored a hangup signal; the terminal that started MACE may have closed. Use mace stop to stop.")
		case sig := <-sigCh:
			log.Printf("[MACE] Caught signal %v, initiating graceful shutdown...", sig)
			break wait
		case err := <-group.Errors():
			log.Printf("[MACE] Unexpected service error: %v, shutting down...", err)
			break wait
		}
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

// installationID names one Mace state directory, so orphan cleanup never touches
// processes that belong to another Mace instance on the same machine.
func installationID(dbPath string) string {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		abs = dbPath
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:8])
}

// maintain prunes old native output hourly and keeps three daily backups.
func maintain(ctx context.Context, store *storage.Store, retention time.Duration) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if _, at := store.LatestBackup(); time.Since(at) > 24*time.Hour {
			if path, e := store.Backup("daily"); e != nil {
				log.Printf("[MACE] Daily backup failed: %v", e)
			} else {
				log.Printf("[MACE] Daily backup written to %s", path)
				store.PruneBackups(3)
			}
		}
		if retention > 0 {
			if n, e := store.PruneEvents(ctx, time.Now().Add(-retention)); e != nil {
				log.Printf("[MACE] Pruning native output failed: %v", e)
			} else if n > 0 {
				log.Printf("[MACE] Pruned %d native output lines older than %s", n, retention)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
