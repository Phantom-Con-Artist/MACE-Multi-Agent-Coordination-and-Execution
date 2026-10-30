# MACE Architecture Specification
**Multi-Agent Coordination and Execution**

---

## 1. Executive Summary & Vision

**MACE** (*Multi-Agent Coordination and Execution*) provides a specialized, robust runtime and transport layer for one machine to remotely coordinate, observe, and control AI agents running on another machine. 

While multi-party mesh networks introduce complex gossip protocols, partition healing overhead, and unbounded state synchronization, MACE focuses on solving the primary developer and infrastructure problem: **one remote machine securely executing multiple AI agents under the strict command of a controlling machine over any network topology.**

```
+------------------------------------+           +------------------------------------+
|          CONTROLLER NODE           |           |          TARGET WORKER NODE        |
|                                    |           |                                    |
| +--------------------------------+ |           | +--------------------------------+ |
| | React UI (Vite / shadcn / TS)  | |           | | Multi-Agent Supervisor Tree    | |
| | - Zustand Connection State     | |           | | - Agent Process A (Claude CLI) | |
| | - TanStack Remote Sync         | |           | | - Agent Process B (Codex CLI)  | |
| | - Apache ECharts Telemetry     | |           | | - Agent Process C (Custom Tool)| |
| +---------------+----------------+ |           | +----------------+---------------+ |
|                 |                  |           |                  |                 |
| +---------------v----------------+ |  libp2p   | +----------------v---------------+ |
| | Orchestrator Engine            | |  Encrypted| | Concurrency Worker Pool        | |
| | - Multi-agent task graph       | |  Yamux    | | - Bounded stream queues        | |
| | - Dispatcher & Heartbeat       | |  Stream   | | - Context cancellation tree    | |
| +---------------+----------------+ | <=======> | +----------------+---------------+ |
|                 |                  | (QUIC/WS/ |                  |                 |
| +---------------v----------------+ |  TCP)     | +----------------v---------------+ |
| | libp2p Host (Controller)       | |           | | libp2p Host (Worker)           | |
| | - Peer ID (Ed25519)            | |           | | - Peer ID (Ed25519)            | |
| | - Hole punching / Relay client | |           | | - Hole punching / Relay client | |
| +---------------+----------------+ |           | +----------------+---------------+ |
|                 |                  |           |                  |                 |
| +---------------v----------------+ |           | +----------------v---------------+ |
| | SQLite Storage (WAL Mode)      | |           | | SQLite Storage (WAL Mode)      | |
| | - Command History & Audit      | |           | | - Execution Trace & State Log  | |
| +--------------------------------+ |           | +--------------------------------+ |
+------------------------------------+           +------------------------------------+
```

---

## 2. Core Architectural Pillars

### 2.1 1-to-1 Remote Machine Topology
- **Deterministic Peer Pairing**: The controller pairs with the target worker through a cryptographically signed invite or pre-shared multiaddr containing the worker's Ed25519 peer ID.
- **Bi-directional Stream Multiplexing**: A single underlying connection carries multiplexed sub-streams:
  - Stream `mace/control/v1`: Session commands, execution dispatches, interrupts, and heartbeats.
  - Stream `mace/events/v1`: Real-time structured agent logs, state transitions, tool invocations, and output chunks.
  - Stream `mace/metrics/v1`: CPU, memory, goroutine counts, and active worker telemetry.

### 2.2 Network Agnosticism
MACE is designed to work whether both machines are on the same home Wi-Fi, across corporate firewalls, or in separate cloud regions:
- **Direct Hole Punching**: Uses libp2p's DCUtR (Direct Connection Upgrade through Relay) and AutoNAT to traverse residential and carrier-grade NATs.
- **Relay Fallback**: When symmetric NAT prevents direct UDP/TCP hole punching, communication automatically routes through a Circuit Relay v2 proxy with zero application reconfiguration.
- **Transport Flexibility**: QUIC is preferred for ultra-fast connection setup and packet loss tolerance; fallback to WebSockets and TCP ensures firewall compatibility.

### 2.3 Individual Multi-Agent Execution Engine
On the worker node, MACE coordinates multiple autonomous agent instances concurrently:
- Each agent runs as an isolated subprocess or supervised goroutine.
- Agents receive distinct working directories, virtual environments, and capability sets.
- The controller can inspect individual agent stdout/stderr streams, pause/resume tasks, or kill hung subagents without affecting adjacent agent instances.

### 2.4 Persistent State & Local Autonomy
- Both controller and worker persist their state in SQLite with WAL (Write-Ahead Logging) enabled.
- If the network connection drops, the worker's agent supervisor continues running the scheduled work locally.
- When the controller reconnects, the worker replays unacknowledged event streams from SQLite to achieve eventual consistency.

---

## 3. Subsystem Breakdown

| Subsystem | Location | Responsibility |
|---|---|---|
| **P2P Transport** | `internal/p2p` | Manages libp2p host lifecycle, peer connection, NAT traversal, and Yamux multiplexers. |
| **Concurrency Core** | `internal/concurrency` | Implements supervisor trees, worker pools, bounded channels, and graceful shutdown contexts. |
| **Orchestrator** | `internal/orchestrator` | Coordinates multi-agent task dispatch, order of execution, and dependency resolution. |
| **Agent Runtime** | `internal/agent` | Spawns, controls, and monitors OS processes and CLI agents with stdin/stdout piping. |
| **Persistence** | `internal/storage` | Manages SQLite schemas, WAL connections, event logs, and audit trails. |
| **API & WebSockets** | `internal/api` | Provides local REST and WebSocket endpoints for the frontend dashboard. |
| **Security Layer** | `internal/security` | Verifies cryptographic identity, signs auth tokens, and validates sandbox boundaries. |
| **Frontend UI** | `web/` | React single-page application with Zustand, TanStack Query, and Apache ECharts. |
