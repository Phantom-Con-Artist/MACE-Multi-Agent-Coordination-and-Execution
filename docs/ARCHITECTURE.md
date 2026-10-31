# MACE Architecture Specification
**Multi-Agent Coordination and Execution**

---

## 1. Executive Summary & Vision

**MACE** (*Multi-Agent Coordination and Execution*) provides a specialized, robust runtime and transport layer for one machine to remotely coordinate, observe, and control AI agents running on another machine. 

Using **Tailscale** (`tsnet`) as its primary networking layer alongside `libp2p`, MACE eliminates network boundaries through an encrypted WireGuard mesh, enabling instant 1-to-1 machine communication without opening firewall ports, requiring root privileges, or configuring complex NAT traversal rules.

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
| +---------------v----------------+ |           | +----------------v---------------+ |
| | Orchestrator Engine            | |           | | Concurrency Worker Pool        | |
| | - Multi-agent task graph       | |           | | - Bounded stream queues        | |
| | - Dispatcher & Heartbeat       | |           | | - Context cancellation tree    | |
| +---------------+----------------+ | Tailscale | +----------------+---------------+ |
|                 |                  | WireGuard |                  |                 |
| +---------------v----------------+ | Mesh / WS | +----------------v---------------+ |
| | Tailscale tsnet / libp2p Host  | | <=======> | | Tailscale tsnet / libp2p Host  | |
| | - Hostname: mace-controller    | | (Encrypted| | - Hostname: mace-worker-1      | |
| | - User-space, No root required | |  Tunnel)  | | - User-space, No root required | |
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

### 2.1 Tailscale WireGuard Mesh & Connection Layer
- **Embedded `tsnet` User-Space Engine**: Runs inside the MACE Go process without requiring root privileges (`sudo`), kernel TUN/TAP interfaces, or a system Tailscale daemon.
- **MagicDNS Addressing**: Controller and worker communicate via predictable human-readable names (`mace-worker-1:8080`).
- **Encrypted Point-to-Point**: Direct peer-to-peer WireGuard tunnels with ChaCha20-Poly1305 encryption.
- **NAT Traversal & DERP Relays**: Traverses NATs and corporate firewalls automatically, with fallback to global DERP relay nodes.
- **Secondary libp2p Transport**: Direct TCP/QUIC hole punching and multi-stream multiplexing for environments without Tailscale credentials.

### 2.2 1-to-1 Remote Machine Topology
- **Deterministic Peer Pairing**: The controller pairs with the target worker through a cryptographically signed invite, pre-shared Tailscale hostname, or multiaddr.
- **Bi-directional Stream Multiplexing**: A single connection carries multiplexed sub-streams:
  - Stream `mace/control/v1`: Session commands, execution dispatches, interrupts, and heartbeats.
  - Stream `mace/events/v1`: Real-time structured agent logs, state transitions, tool invocations, and output chunks.
  - Stream `mace/metrics/v1`: CPU, memory, goroutine counts, and active worker telemetry.

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
| **Tailscale Network** | `server/internal/network/tailscale` | Manages embedded `tsnet.Server` lifecycle, WireGuard auth, and MagicDNS listeners. |
| **P2P Transport** | `server/internal/p2p` | Manages secondary libp2p host lifecycle, peer connection, NAT traversal, and Yamux multiplexers. |
| **Concurrency Core** | `server/internal/concurrency` | Implements supervisor trees, worker pools, bounded channels, and graceful shutdown contexts. |
| **Orchestrator** | `server/internal/orchestrator` | Coordinates multi-agent task dispatch, order of execution, and dependency resolution. |
| **Agent Runtime** | `server/internal/agent` | Spawns, controls, and monitors OS processes and CLI agents with stdin/stdout piping. |
| **Persistence** | `server/internal/storage` | Manages SQLite schemas, WAL connections, event logs, and audit trails. |
| **API & WebSockets** | `server/internal/api` | Provides local REST and WebSocket endpoints for the frontend dashboard. |
| **Security Layer** | `server/internal/security` | Verifies cryptographic identity, signs auth tokens, and validates sandbox boundaries. |
| **Frontend UI** | `web/` | React single-page application with Zustand, TanStack Query, and Apache ECharts. |
