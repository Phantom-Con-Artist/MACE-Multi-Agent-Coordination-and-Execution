# MACE — Machine Autonomous Control & Execution

> **1-to-1 Remote Machine AI Agent Orchestration (Network Agnostic)**

MACE is a streamlined, resilient system designed to allow one machine to remotely control and coordinate AI agent execution on another machine. Drawing architectural inspiration from [RAIN](../project-rain), MACE simplifies the topological complexity into a dedicated, robust **1-to-1 peer relationship** while emphasizing high concurrency, protocol reliability, process security, and complete network agnosticism.

---

## Core Philosophy

- **1-to-1 Remote Control**: A primary controller paired with a remote worker machine executing AI agents and terminal tasks.
- **Network Agnostic**: Operates seamlessly across local LAN, NATs, firewalls, and WAN through libp2p transports (TCP, QUIC, WebSocket) and hole punching.
- **Concurrency & Robustness**: Resilient goroutine worker pools, actor-style supervisors, backpressure control, and context-bounded cancellation.
- **Security & Isolation**: Mutual cryptographic peer identity verification, capability-based command gating, and sandboxed agent execution.

---

## Tech Stack

### Backend (Go)
- **Language**: Go (v1.26+)
- **Networking & P2P**: [`go-libp2p`](https://github.com/libp2p/go-libp2p) (TCP, QUIC, WebSockets, Circuit Relay v2, NAT hole punching)
- **HTTP & Streaming**: [`go-chi/chi`](https://github.com/go-chi/chi) & `gorilla/websocket` (or `nhooyr.io/websocket`)
- **Database**: SQLite (`modernc.org/sqlite` or `mattn/go-sqlite3`) for state persistence, audit trail, and session logs
- **Concurrency**: Go standard sync, errgroup, worker pools, supervisor channels

### Frontend (React / Web UI)
- **UI Framework**: React (Vite / TypeScript)
- **Styling & Components**: Tailwind CSS, [`shadcn/ui`](https://ui.shadcn.com/)
- **State Management**: [Zustand](https://github.com/pmndrs/zustand)
- **Data Fetching & Caching**: TanStack Query (React Query)
- **Data Visualization & Metrics**: Apache ECharts (`echarts-for-react`)

---

## Workspace Layout

```
project-mace/
├── cmd/                    # Application Entrypoints
│   ├── maced/             # MACE Daemon (Controller / Worker Node)
│   └── mactl/             # MACE CLI control tool
├── internal/               # Private application and library code
│   ├── agent/             # Agent execution runtime & process isolation
│   ├── api/               # HTTP REST & WebSocket API endpoints (go-chi)
│   ├── concurrency/       # Supervisors, worker pools & lifecycle primitives
│   ├── config/            # Configuration management
│   ├── orchestrator/      # 1-to-1 remote agent coordination engine
│   ├── p2p/               # libp2p host, stream multiplexing & transport
│   ├── security/          # Peer identity, token verification & sandboxing
│   └── storage/           # SQLite persistence layer & migrations
├── pkg/                    # Public packages & shared wire protocols
│   └── protocol/          # Protobuf / JSON-RPC message schemas & events
├── web/                    # React frontend application
├── docs/                   # Documentation & architectural specs
├── scripts/                # Development, build, and deployment automation
├── .editorconfig           # Editor formatting definitions
├── .env.example            # Environment configuration template
├── .gitattributes          # Git LF normalization & binary rules
├── .gitignore              # Ignored files (Go, Node, SQLite, secrets)
└── README.md
```

---

## Current Status

- [x] Environment and repository initialized.
- [x] Git attributes, ignores, and workspace scaffolding configured.
- [ ] Protocol and message envelope definition (Phase 1).
- [ ] libp2p host & transport implementation (Phase 2).
- [ ] SQLite persistence & state store (Phase 3).
- [ ] Agent execution & concurrency engine (Phase 4).
- [ ] React UI (Tailwind, shadcn, Zustand, ECharts) (Phase 5).
