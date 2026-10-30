# MACE — Multi-Agent Coordination and Execution

> **1-to-1 Remote Machine Multi-Agent Orchestration & Control (Tailscale & Network Agnostic)**

**MACE** (*Multi-Agent Coordination and Execution*) is a high-performance system designed to enable **one controller machine to remotely manage, coordinate, and execute multiple AI agents on a target worker machine**.

Powered by **Tailscale** (`tsnet`) and `libp2p`, MACE establishes an instant, encrypted, zero-configuration WireGuard mesh between machines over any network topology (NATs, firewalls, public/private clouds) without port forwarding or root privileges.

---

## Key Pillars

1. **Tailscale Zero-Config Networking**:
   - Embedded user-space Tailscale node (`tailscale.com/tsnet`). No `sudo`, no kernel TUN/TAP driver, and no system daemon required.
   - Point-to-point WireGuard mesh encryption with automatic NAT traversal and DERP relay fallback.
   - Direct connection via MagicDNS (e.g. `mace-worker-1:8080`).
2. **Multi-Agent Coordination & Execution**:
   - Controller sends high-level tasks, orchestrates agent dependencies, and streams bidirectional interactions.
   - Target worker machine executes individual agents concurrently in isolated supervisor pools with bounded context and resource limits.
3. **Robust Concurrency Engine**:
   - Supervisor trees and worker pools written in Go (`errgroup`, `sync`, bounded channels).
   - Backpressure management prevents memory leaks during high-throughput stdout/stderr log bursts.
   - Context-bounded cancellation ensures no orphaned CLI or subagent processes.
4. **Defense-in-Depth Security**:
   - WireGuard point-to-point cryptographic encryption and Tailnet ACL tags.
   - HMAC session tokens for local API endpoints.
   - Granular execution permissions and sandboxed working directories.
   - Append-only SQLite audit log recording every remote instruction, tool execution, and state change.

---

## Technology Stack

| Layer | Technologies & Dependencies | Purpose |
|---|---|---|
| **Core & Daemon** | Go 1.26+ | Fast, statically compiled, low-overhead daemon |
| **Networking & Mesh** | [`tailscale.com/tsnet`](https://tailscale.com) & `go-libp2p` | User-space WireGuard mesh, MagicDNS, DERP fallback, zero-config P2P |
| **HTTP & WebSockets** | `go-chi/chi/v5`, `gorilla/websocket` | REST control API, real-time event & terminal streaming |
| **Concurrency Primitives** | `golang.org/x/sync` (`errgroup`, `semaphore`) | Actor-style process supervisors, worker pools, bounded streams |
| **Persistence & Audit** | SQLite (`modernc.org/sqlite` pure Go) | Session state, task history, audit trails (WAL mode) |
| **Web UI Framework** | React 19, TypeScript, Vite | Modern, snappy single-page dashboard |
| **Design & UI Kit** | Tailwind CSS v4, `shadcn/ui` foundation | Polished visual interface |
| **Iconography** | `lucide-react`, `lucide-static` SVG assets | Consistent, modern visual system |
| **State & Caching** | Zustand, TanStack Query (React Query) | Synchronized local store & query caching |
| **Metrics & Telemetry** | Apache ECharts (`echarts-for-react`) | Real-time agent concurrency, latency, and host resource telemetry |

---

## Directory Structure

```
project-mace/
├── cmd/                          # Executable entrypoints
│   ├── maced/                   # Main MACE daemon (Controller or Worker)
│   └── mactl/                   # Administrative CLI tool
├── internal/                     # Private application core
│   ├── agent/                   # Agent process runtime & CLI executors
│   ├── api/                     # go-chi REST API & WebSocket handlers
│   ├── concurrency/             # Supervisor trees, worker pools, queues
│   ├── config/                  # Node & network configuration
│   ├── network/                 # Tailscale & connection layer
│   │   └── tailscale/           # Embedded tsnet user-space integration
│   ├── orchestrator/            # Multi-agent coordination engine
│   ├── p2p/                     # libp2p host, multiaddr routing, NAT traversal
│   ├── security/                # Peer identity, token verification, sandboxing
│   └── storage/                 # SQLite migrations, store & audit log
├── pkg/                          # Public protocols & shared libraries
│   └── protocol/                # Wire envelopes, event schemas, frame codecs
├── web/                          # React client application
│   ├── public/icons/            # Supportive Lucide SVG icons
│   ├── package.json             # React, Tailwind, Lucide, Zustand, ECharts
│   └── ...
├── docs/                         # System specifications & technical guides
│   ├── ARCHITECTURE.md          # End-to-end system architecture
│   ├── TAILSCALE.md             # Tailscale networking, tsnet, and ACL guide
│   ├── CONCURRENCY.md           # Concurrency model, pools & backpressure
│   ├── PROTOCOLS.md             # Wire protocol & WebSocket frame specs
│   └── SECURITY.md              # Threat model, auth, and sandboxing
├── scripts/                      # Build, test, and release scripts
├── .editorconfig                 # Editor styling rules
├── .env.example                  # Environment configuration template
├── .gitattributes                # LF normalization and binary attributes
├── .gitignore                    # Go, Node, SQLite, secrets rules
├── go.mod                        # Go module definition (tailscale, libp2p, chi, sqlite)
└── README.md
```

---

## Documentation Index

- [Tailscale Networking Guide](file:///home/subhradeep/Development/project-mace/docs/TAILSCALE.md)
- [Architecture Blueprint](file:///home/subhradeep/Development/project-mace/docs/ARCHITECTURE.md)
- [Concurrency & Supervision Model](file:///home/subhradeep/Development/project-mace/docs/CONCURRENCY.md)
- [Protocols & Framing Specification](file:///home/subhradeep/Development/project-mace/docs/PROTOCOLS.md)
- [Security & Threat Model](file:///home/subhradeep/Development/project-mace/docs/SECURITY.md)
