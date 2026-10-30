# MACE Concurrency & Orchestration Architecture

MACE (*Multi-Agent Coordination and Execution*) is engineered around resilient Go concurrency primitives. Coordinating multiple autonomous AI agents on a remote worker machine demands rigorous control over goroutine lifetimes, channel buffering, process management, and cancellation boundaries.

---

## 1. Concurrency Model Principles

1. **No Leaked Goroutines**: Every goroutine belongs to an explicit lifecycle owner (Supervisor or Task Runner) and receives a cancellable `context.Context`.
2. **Deterministic Process Cancellation**: Cancelling an agent task immediately cascades to OS process tree termination (`SIGTERM` followed by `SIGKILL` timeout).
3. **Explicit Backpressure**: Channels between agent stdout/stderr pipes, internal event buses, and the P2P wire are bounded. When buffers fill, non-blocking dropping or deliberate backpressure throttling is applied rather than unbounded memory expansion.
4. **Isolated Agent Failure Domains**: The failure, crash, or memory exhaustion of one agent must never terminate or corrupt sibling agents or the host MACE daemon.

---

## 2. Supervisor Tree Architecture

MACE implements an actor-inspired supervisor pattern in `internal/concurrency/supervisor.go`:

```
                       +-------------------+
                       |  Root Supervisor  |
                       +---------+---------+
                                 |
        +------------------------+------------------------+
        |                                                 |
+-------v---------+                              +--------v--------+
| P2P Transport   |                              | Agent Pool      |
| Supervisor      |                              | Supervisor      |
+-------+---------+                              +--------+--------+
        |                                                 |
        |-- Connection Keeper                             |-- Agent Instance A (Worker 1)
        |-- Relay Heartbeat                               |-- Agent Instance B (Worker 2)
        |-- Hole Punch Monitor                            |-- Agent Instance C (Worker 3)
```

### Supervisor Restart Strategies:
- **One-For-One**: If an individual agent crashes, only that specific agent runner is inspected and either restarted or moved to an `ERRORED` state with captured diagnostics.
- **Backoff Throttling**: Exponential backoff prevents crash-loops from consuming 100% CPU when an external CLI or tool binary is misconfigured.

---

## 3. Worker Pools & Task Queuing

The worker pool (`internal/concurrency/pool.go`) schedules and limits concurrent agent execution:

- **Configurable Concurrency Cap**: `MACE_MAX_CONCURRENT_AGENTS` dictates how many active processes can run simultaneously.
- **Task Prioritization**: Incoming commands are prioritized into:
  - `HIGH`: Interrupts, pause, kill, heartbeat signals.
  - `NORMAL`: Standard agent prompt dispatch, tool invocation.
  - `LOW`: Historical log sync, telemetry exports.
- **Context Boundaries**: Each worker executes with a derived context that enforces:
  ```go
  ctx, cancel := context.WithTimeout(parentCtx, agentTimeout)
  defer cancel()
  ```

---

## 4. Backpressure & Stream Buffering

AI coding agents can generate massive volumes of terminal output and diffs in milliseconds. MACE protects network buffers and memory via:

- **Ring Buffer Caching**: Local agent outputs are written to fixed-size in-memory ring buffers (e.g., 2MB per session).
- **Batching & Debouncing**: High-frequency output chunks are consolidated into unified protocol frames before transmission across libp2p streams.
- **Drop Policies on Telemetry**: Metrics and non-critical status updates use lossy dropping under severe network congestion, while command control streams strictly enforce reliable TCP/QUIC delivery.

---

## 5. Synchronization & Race Prevention

- **Mutex Encapsulation**: Shared state structs (`SessionManager`, `NodeRegistry`) use private fields protected by `sync.RWMutex`.
- **Atomic Counters**: High-frequency metrics (bytes transferred, active goroutines, message sequence IDs) use `sync/atomic` to avoid lock contention.
- **Race Detector Mandate**: All unit and integration tests must pass `go test -race ./...` before merge.
