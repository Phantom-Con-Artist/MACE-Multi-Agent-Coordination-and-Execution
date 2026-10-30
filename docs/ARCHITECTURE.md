# MACE Architecture Specification

## 1. Overview & Vision

**MACE** (*Machine Autonomous Control & Execution*) is a 1-to-1 remote machine AI agent orchestrator. Where large networks handle multi-tenant mesh clouds, MACE concentrates on the direct peer link: **one controller machine remotely commanding, observing, and coordinating AI agents on another target machine.**

```
+--------------------+                       +--------------------+
|  Controller Node   |                       |    Worker Node     |
|                    |     libp2p Tunnel     |                    |
| +----------------+ | (QUIC / WS / Relay)   | +----------------+ |
| | React UI + Web | | <===================> | | Agent Runtime  | |
| | (Zustand/Query)| |                       | | (CLI / Process)| |
| +--------+-------+ |                       | +--------+-------+ |
|          |         |                       |          |         |
| +--------v-------+ |                       | +--------v-------+ |
| | Orchestrator   | |                       | | Concurrency    | |
| | Engine (Go)    | |                       | | Engine (Go)    | |
| +--------+-------+ |                       | +--------+-------+ |
|          |         |                       |          |         |
| +--------v-------+ |                       | +--------v-------+ |
| | SQLite Storage | |                       | | SQLite Storage | |
| +----------------+ |                       | +----------------+ |
+--------------------+                       +--------------------+
```

---

## 2. Network-Agnostic P2P Layer (`internal/p2p`)

MACE leverages `go-libp2p` to eliminate network boundaries:
1. **Transports**: Native support for QUIC, TCP, and WebSockets.
2. **NAT Traversal & Hole Punching**: DCUtR (Direct Connection Upgrade through Relay) and AutoNAT to punch through residential NATs and firewalls without port forwarding.
3. **Relay Fallback**: Circuit Relay v2 fallback ensures connectivity even under symmetric NAT constraints.
4. **Multiplexing & Security**: Noise handshake / TLS 1.3 protocol encryption, yamux stream multiplexing. Single TCP/QUIC connection carries multiple bi-directional data streams (logs, terminal I/O, control events).

---

## 3. Concurrency & Orchestration Model (`internal/concurrency`)

To manage AI agents reliably without unbounded resource leaks:
- **Supervisor Tree**: Erlang-style supervisor pattern in Go. If an agent process crashes or hangs, the supervisor handles backoff restarts, state capture, or graceful tear-down.
- **Worker Pools & Bounded Channels**: Fixed-capacity buffered channels prevent memory bloat during high-output agent stdout/stderr bursts.
- **Context Boundaries**: Hierarchical Go `context.Context` propagation ensures deterministic cancellation cascades when an operation is cancelled or a connection drops.
- **Backpressure & Rate Limiting**: Token-bucket rate limiting on remote RPC calls and log streaming.

---

## 4. Security & Isolation (`internal/security`)

- **Peer Identity**: Cryptographic Ed25519 public keys identify both machines. Only authorized peer IDs are allowed to establish streams.
- **Session Tokens**: Short-lived, HMAC-signed bearer tokens for local WebSocket & REST endpoints.
- **Execution Boundaries**: Remote tool and bash execution run with defined environment constraints, timeouts, and working directory sandboxes.
- **Audit Logging**: Every command invocation, state change, and agent action is persisted in SQLite with an immutable timestamped log.

---

## 5. Persistence & Storage (`internal/storage`)

- **Engine**: SQLite in WAL (Write-Ahead Logging) mode.
- **Entities**:
  - `nodes`: Controller and worker peer profiles and keys.
  - `sessions`: Active and historical agent orchestration runs.
  - `tasks`: Individual steps dispatched to remote agents.
  - `events`: Streamed log outputs, metrics, and error traces.

---

## 6. Frontend Telemetry & Control (`web/`)

- **React + Tailwind + shadcn/ui**: High-performance, clean responsive dashboard.
- **Zustand**: Client-side state store for real-time peer connection status and active agent tabs.
- **TanStack Query**: Remote query caching and synchronization.
- **Apache ECharts**: Live concurrency metrics, token usage, latency, and CPU/memory utilization of the remote agent host.
