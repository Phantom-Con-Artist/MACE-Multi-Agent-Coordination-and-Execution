# MACE — Multi-Agent Coordination and Execution

> **1-to-1 Remote Machine Multi-Agent Orchestration & Control (Network Agnostic)**

**MACE** (*Multi-Agent Coordination and Execution*) is a high-performance, network-agnostic system designed to enable **one controller machine to remotely manage, coordinate, and execute multiple AI agents on a target worker machine**.

Inspired by [RAIN](../project-rain), MACE shifts the architecture from complex multi-party meshes to an ultra-resilient, dedicated **1-to-1 machine-to-machine control plane**. On the target machine, MACE supervises individual AI agents with industrial-strength concurrency, stream isolation, deterministic process cancellation, and cryptographic security.

---

## Key Pillars

1. **Multi-Agent Coordination & Execution**:
   - Controller sends high-level tasks, orchestrates agent dependencies, and streams bidirectional interactions.
   - Target machine executes agents independently in dedicated supervisor pools with isolated context and resource limits.
2. **Network Agnostic (Zero-Config Connectivity)**:
   - Built on `go-libp2p`: connects across local LAN, strict NATs, dynamic IPs, and WAN without opening firewall ports.
   - Hole punching (DCUtR / AutoNAT) with seamless Circuit Relay v2 fallback.
   - Dual transport support: QUIC, WebSockets, and TCP with Yamux multiplexing.
3. **Robust Concurrency Engine**:
   - Supervisor trees and worker pools written in Go (`errgroup`, `sync`, bounded channels).
   - Backpressure management prevents memory leaks during high-throughput stdout/stderr log bursts.
   - Context-bounded cancellation ensures no orphaned CLI or subagent processes.
4. **Defense-in-Depth Security**:
   - Ed25519 cryptographic peer identities (Noise / TLS 1.3 channel encryption).
   - HMAC session tokens for local API endpoints.
   - Granular execution permissions and sandboxed working directories.
   - Append-only SQLite audit log recording every remote instruction, tool execution, and state change.

---

## Technology Stack

| Layer | Technologies & Dependencies | Purpose |
|---|---|---|
| **Core & Daemon** | Go 1.26+ | Fast, statically compiled, low-overhead daemon |
| **P2P Networking** | `go-libp2p`, `quic-go`, `yamux` | NAT hole punching, secure peer identity, multi-stream multiplexing |
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
│   ├── CONCURRENCY.md           # Concurrency model, pools & backpressure
│   ├── PROTOCOLS.md             # Wire protocol & WebSocket frame specs
│   └── SECURITY.md              # Threat model, auth, and sandboxing
├── scripts/                      # Build, test, and release scripts
├── .editorconfig                 # Editor styling rules
├── .env.example                  # Environment configuration template
├── .gitattributes                # LF normalization and binary attributes
├── .gitignore                    # Go, Node, SQLite, secrets rules
├── go.mod                        # Go module definition
└── README.md
```

---

## Documentation Index

- [Architecture Blueprint](file:///home/subhradeep/Development/project-mace/docs/ARCHITECTURE.md)
- [Concurrency & Supervision Model](file:///home/subhradeep/Development/project-mace/docs/CONCURRENCY.md)
- [Protocols & Framing Specification](file:///home/subhradeep/Development/project-mace/docs/PROTOCOLS.md)
- [Security & Threat Model](file:///home/subhradeep/Development/project-mace/docs/SECURITY.md)
