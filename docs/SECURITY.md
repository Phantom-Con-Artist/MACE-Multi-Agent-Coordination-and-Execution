# MACE Security & Isolation Model

Security in MACE (*Multi-Agent Coordination and Execution*) is built from the assumption that the controller and worker communicate over untrusted public networks and execute potentially arbitrary code on the target host.

---

## 1. Cryptographic Identity & Transport Security

- **Ed25519 Peer Keys**: Each MACE node generates an Ed25519 keypair on first initialization stored in `.mace/identity.key`. The cryptographic peer ID is derived from this public key.
- **Mutual Authentication**: Connection attempts are rejected immediately unless the remote peer ID exists in the node's authorized peers list (`allowed_peers`).
- **Encrypted Transport**: All streams are encrypted using the libp2p Noise protocol (`/noise`) or TLS 1.3. Eavesdropping, man-in-the-middle attacks, and stream tampering are mathematically prevented.

---

## 2. Remote Command Gating & Sandboxing

Allowing a remote machine to spawn CLI agents creates severe security risks without strict isolation:

1. **Path Whitelisting**: The target worker defines an approved list of executable binaries (e.g., `claude`, `codex`, `git`, `python`, `node`). Arbitrary shell command execution can be restricted to whitelisted binary envelopes.
2. **Directory Sandboxing**: Each agent instance is assigned an explicit, contained working directory. Path traversal (`../..`) outside the configured workspace root is prohibited.
3. **Resource Quotas**:
   - Memory limits enforced via Linux cgroups (or OS process monitors).
   - CPU throttling to prevent 100% core exhaustion.
   - Hard execution timeouts (`timeout_seconds`) with automatic `SIGKILL` cleanup.
4. **Environment Isolation**: Sensitive environment variables on the worker host (e.g., AWS credentials, SSH keys, master DB credentials) are scrubbed before launching child agent processes.

---

## 3. Local API Security (Web Client)

- **Loopback Binding**: The `go-chi` REST and WebSocket servers bind to `127.0.0.1` by default.
- **HMAC Bearer Tokens**: Web client requests require an authorization header:
  ```http
  Authorization: Bearer <mace_token>
  ```
- **Strict CORS & CSRF**: Cross-Origin requests are restricted to the local Vite dev server and configured production domains.

---

## 4. Immutable Audit Trail

Every operational event is recorded in the local SQLite database in WAL mode:
- Timestamp (nanosecond precision)
- Peer ID of dispatcher
- Agent ID and command invoked
- Exit code and error summary
- Session termination reason

Audit logs cannot be modified via remote API commands.
