# Tailscale Networking in MACE

MACE (*Multi-Agent Coordination and Execution*) utilizes **Tailscale** as its primary networking and connection backbone for 1-to-1 remote machine control and AI agent orchestration.

---

## 1. Why Tailscale for MACE?

Connecting two arbitrary machines (e.g., a developer's MacBook at a coffee shop and a powerful desktop workstation or cloud GPU box at home/office) typically runs into firewall boundaries, symmetric NATs, dynamic IPs, and complex port-forwarding setups.

Tailscale solves this with:
1. **Zero-Config Mesh (WireGuard)**: Every machine receives a stable, private 100.x.y.z IP and a friendly **MagicDNS** name (e.g., `mace-worker-1`).
2. **Encrypted Point-to-Point**: Direct peer-to-peer WireGuard tunnels with ChaCha20-Poly1305 encryption. Traffic never leaves the private Tailnet.
3. **DERP Relay Fallback**: If two endpoints are behind aggressive symmetric NATs, Tailscale seamlessly relays encrypted WireGuard packets via DERP (Designated Encrypted Relay for Packets) servers.
4. **Automatic HTTPS Certificates**: Embedded Let's Encrypt certificates for nodes on your Tailnet.

---

## 2. Embedded `tsnet` Architecture (Zero Root, Zero Daemon)

MACE integrates Tailscale via **`tailscale.com/tsnet`**, Tailscale's official embedded user-space Go library.

```
+------------------------------------+           +------------------------------------+
|          CONTROLLER NODE           |           |          TARGET WORKER NODE        |
|                                    |           |                                    |
| +--------------------------------+ |           | +--------------------------------+ |
| | React UI (Zustand / TanStack)  | |           | | Multi-Agent Supervisor Tree    | |
| +---------------+----------------+ |           | +----------------+---------------+ |
|                 |                  |           |                  |                 |
| +---------------v----------------+ |           | +----------------v---------------+ |
| | go-chi HTTP / WS Client        | |           | | go-chi HTTP / WS Server        | |
| +---------------+----------------+ |           | +----------------+---------------+ |
|                 |                  |           |                  |                 |
| +---------------v----------------+ | WireGuard | +----------------v---------------+ |
| | tsnet.Server / Client (Go)     | <===========> | tsnet.Server (Go Embedded)     | |
| | Hostname: mace-controller      |  Encrypted  | | Hostname: mace-worker-1        | |
| | (User-Space, No root needed)   |   Tunnel    | | (User-Space, No root needed)   | |
| +--------------------------------+ |           | +--------------------------------+ |
+------------------------------------+           +------------------------------------+
                                      \         /
                                       \       /
                            +-----------v-----v-----------+
                            | Tailscale Coordination Plane|
                            | (DERP / MagicDNS / Keys)    |
                            +-----------------------------+
```

### Key Advantages of Embedded `tsnet`:
- **No System Daemon Needed**: Operates entirely in user-space inside the `maced` process.
- **No Root Privileges**: Runs without `sudo` and without modifying host TUN/TAP network interfaces.
- **Cross-Platform**: Operates identically on Linux, macOS, and Windows.
- **Ephemeral Node Cleanup**: Nodes can automatically deregister from your Tailnet when terminated (`MACE_TS_EPHEMERAL=true`).

---

## 3. Configuration & Environment Variables

All Tailscale settings are managed via environment variables (see [`.env.example`](../.env.example)):

```bash
# Network Mode
MACE_NETWORKING_MODE=tailscale

# Tailscale Auth Key (Reusable or Ephemeral recommended)
# Generate from: https://login.tailscale.com/admin/settings/keys
MACE_TS_AUTHKEY=tskey-auth-kXXXXX-XXXXXXXXXXXXXXXXXXXXXXXXXXXX

# Tailscale Machine Hostname on your Tailnet
MACE_TS_HOSTNAME=mace-worker-1

# State storage for node private keys and WireGuard identity
MACE_TS_STATE_DIR=./data/tailscale

# Ephemeral node (auto-remove from admin console on daemon stop)
MACE_TS_EPHEMERAL=true

# Remote Target (Used by Controller to dial Worker)
MACE_REMOTE_TS_HOST=mace-worker-1
MACE_REMOTE_TS_PORT=8080
```

---

## 4. Connection Workflow

1. **Worker Launch**:
   - `maced --role worker` initializes a `tsnet.Server`.
   - The worker authenticates with the provided `MACE_TS_AUTHKEY`.
   - The worker starts listening on `mace-worker-1:8080` (only reachable within the Tailnet).
2. **Controller Pairing**:
   - `maced --role controller` connects to the Tailnet.
   - The controller dials `http://mace-worker-1:8080/api/v1/ws` over the WireGuard mesh.
   - Mutual cryptographic identity is verified, and bi-directional agent orchestration streams begin.
3. **Multi-Agent Execution**:
   - Prompts, tool calls, bash executions, and real-time logs flow securely across the 1-to-1 tunnel.

---

## 5. Security & Access Control (ACLs)

In the Tailscale Admin Console, you can enforce strict ACL policies between your controller and worker nodes using tags:

```json
{
  "tagOwners": {
    "tag:mace-controller": ["autogroup:admin"],
    "tag:mace-worker": ["autogroup:admin"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["tag:mace-controller"],
      "dst": ["tag:mace-worker:8080"]
    }
  ]
}
```

This guarantees that only authorized controller machines can connect to MACE worker agent runtimes.
