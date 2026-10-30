# MACE Protocol & Wire Specification

This document specifies the communication protocols, framing format, and message types used across MACE nodes and between the daemon and web clients.

---

## 1. libp2p Protocol Multiplexing

MACE establishes dedicated Yamux multi-stream channels over an encrypted libp2p connection:

| Protocol ID | Purpose | Delivery Semantics |
|---|---|---|
| `/mace/handshake/1.0.0` | Initial capability negotiation, peer authentication, and role assertion | Strict request/response |
| `/mace/control/1.0.0` | Agent lifecycle commands (spawn, input, pause, terminate) | Ordered, reliable RPC |
| `/mace/stream/1.0.0` | Bi-directional agent stdout, stderr, and interactive stdin piping | Ordered streaming |
| `/mace/telemetry/1.0.0` | Periodic host CPU/RAM metrics, goroutine counts, task latency | Periodic best-effort |

---

## 2. Wire Envelope Format

Every message on the `/mace/control/1.0.0` and `/mace/stream/1.0.0` streams is length-prefixed:

```
+-----------------------------------------------------------------+
| Length (4 Bytes, Big Endian) | JSON/Protobuf Payload Body (N B) |
+-----------------------------------------------------------------+
```

### 2.1 Generic Message Envelope Schema

```json
{
  "version": "1.0.0",
  "id": "msg_01J9...",
  "trace_id": "trc_88A...",
  "type": "SPAWN_AGENT_REQ | AGENT_OUTPUT | INTERRUPT | HEARTBEAT",
  "sender_peer_id": "12D3KooW...",
  "timestamp_ns": 1730302800000000000,
  "payload": {}
}
```

---

## 3. Control Messages (`/mace/control/1.0.0`)

### 3.1 Spawn Agent Request (`SPAWN_AGENT_REQ`)
```json
{
  "agent_id": "agent_alpha_01",
  "name": "Claude Code Worker",
  "command": "claude",
  "args": ["--verbose"],
  "env": {
    "WORKSPACE_DIR": "/tmp/workspace-1"
  },
  "working_dir": "/tmp/workspace-1",
  "max_memory_mb": 1024,
  "timeout_seconds": 3600
}
```

### 3.2 Spawn Agent Response (`SPAWN_AGENT_RESP`)
```json
{
  "agent_id": "agent_alpha_01",
  "pid": 284102,
  "status": "RUNNING",
  "created_at": "2026-10-30T21:30:00Z"
}
```

### 3.3 Interrupt / Kill Request (`INTERRUPT_REQ` / `KILL_REQ`)
```json
{
  "agent_id": "agent_alpha_01",
  "signal": "SIGINT | SIGTERM | SIGKILL",
  "reason": "User requested abort"
}
```

---

## 4. Streaming Output Messages (`/mace/stream/1.0.0`)

### 4.1 Output Chunk (`AGENT_OUTPUT_CHUNK`)
```json
{
  "agent_id": "agent_alpha_01",
  "sequence": 1420,
  "stream": "stdout | stderr",
  "data_b64": "SGVsbG8gV29ybGQK",
  "is_eof": false
}
```

---

## 5. Web Client WebSocket Protocol

The local MACE daemon exposes a WebSocket endpoint at `ws://127.0.0.1:8080/api/v1/ws` for the React dashboard:

- **Subscribe to Agent**: `{"action": "subscribe", "agent_id": "agent_alpha_01"}`
- **Send Terminal Keystrokes**: `{"action": "input", "agent_id": "agent_alpha_01", "data": "ls -la\n"}`
- **Live Metrics Feed**: Telemetry pushes containing CPU, memory, and active agent states for Apache ECharts rendering.
