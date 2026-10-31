# MACE design: hub and worker nodes

Status: direction approved by the owner on 2026-10-31. Steps 1 (records), 2 (worker service), and 3 (joining) built. Date: 2026-10-31.

This document replaces the peer model (one host connecting to another with a URL and token) and the host-facing parts of phase 4 in [DESIGN-REMOTE-COWORKERS.md](DESIGN-REMOTE-COWORKERS.md). Everything else in that document still applies. AGENTS.md constraints still apply.

## The rule

Every node stores its own data. A node is the only writer of what it owns. Other nodes may keep read-only cached copies, marked as cached, so history stays readable while the owner node is away.

## Roles

One daemon, two roles.

| Role | What it does | How it runs |
|---|---|---|
| Worker | Runs agents, owns their private chats and native sessions, owns the groups it hosts, serves files, launches apps, samples machine metrics | `mace` as a user service, started at boot, restarted on failure |
| Hub | Discovers workers, merges their data into one view, routes the owner's actions to the owning worker, serves the web interface | `mace-server start` on the device the owner is using; any worker can also serve it |

A hub device is also a worker. The hub stores no data of its own beyond caches, so switching hubs changes nothing the owner sees.

The phone is never a node. It opens a hub's web interface over HTTPS and can install it as a home-screen app. No dedicated mobile application. A phone can use any always-on worker as its hub, so the laptop does not need to be awake.

## Ownership

| Data | Owner |
|---|---|
| An agent, its profile, model, folder, native session | The worker it runs on |
| A private chat with an agent | That agent's worker |
| A group's settings: name, members, roles, mode, goal | The group's home worker, chosen at creation (default: the first always-on worker) |
| A message | The node that wrote it: the agent's worker for agent messages, the conversation's owner for the owner's messages |
| Receipts and read cursors | The node of the reader |
| Files | The node where the file was created or attached |

The group's home worker runs the group's turns, so groups keep working when the hub closes. Agents on other workers take turns through their own worker.

## Records

Everything stored is a record: a message, a group settings change, a membership change, a receipt, an edit, a deletion marker, a file manifest.

### Identity by hash

A record's ID is the SHA-256 of its canonical envelope:

| Field | Purpose |
|---|---|
| `node` | ID of the writing node |
| `author` | Agent, owner, or MACE |
| `conversation` | Private chat or group ID |
| `clock` | Hybrid logical clock: wall time plus a counter, never moving backwards |
| `prev` | Hash of this node's previous record in this conversation |
| `seen` | Hashes of the newest records from other nodes the author had seen |
| `kind` | message, note, settings, member, receipt, edit, delete, file |
| `body` | The content |
| `sig` | The writing node's signature over the fields above |

Identical text produces different IDs because the envelope differs. A record cannot change without its ID changing, so a fetched copy proves itself. The canonical encoding is fixed and versioned so hashes stay stable across releases.

Node IDs are the hash of the node's public key, created when the node is set up.

### Chains

Each node's records in a conversation form a hash chain through `prev`. A node's head hash per conversation summarizes everything it has written there. Breaks in the chain show missing or altered records.

### Ordering

Records are shown in causal order: a record always follows everything in its `seen` and `prev`. Ties are broken by `clock`, then by node ID. Clock skew between machines cannot place a reply before the message it answers.

### Edits and deletion

An edit is a new record naming the record it replaces. A deletion is a marker naming the deleted hash. Caches drop the content when they sync the marker. The owner node decides.

## Sync

1. The hub asks each available worker for its head hash per conversation.
2. Heads that match the cache need nothing.
3. For a differing head, the hub walks the chain back from the new head until it reaches a known hash, and fetches only those records.
4. Every fetched record is verified: hash matches content, signature matches the node's enrolled key, `prev` links to a known record.
5. Records from an unavailable node are served from cache, marked as cached. Records never cached show as a plain gap: "PC is offline. Its messages appear when it returns."

There are no merge conflicts, because each node is the only writer of its own chain.

Reconstruction restores the record, not what agents knew at the time. Agents that kept working while a node was away did not see its messages, and MACE does not re-run their turns.

## Files

Files are split into chunks. Each chunk is addressed by its SHA-256. A file's ID is the hash of its manifest: the ordered chunk hashes, size, name, and type. Transfers fetch missing chunks only, so they resume after interruption, verify every chunk, and have no size cap. A file shared twice is stored once. This covers the transfer part of phase 3.

## Joining and availability

- Tailscale proves the device belongs to the owner. MACE does not run its own network.
- `mace join` on a new machine, or scanning a QR code from a hub, records the new node's public key with the existing nodes. Joining happens once per machine.
- Hubs find workers through the tailnet and the joined-node list. No addresses are typed.
- A worker that is reachable and healthy is available. Asleep, off, or unreachable is unavailable. Nothing more.
- A locked screen makes only desktop app launching unavailable. Agents and files keep working.

## What changes from today

| Today | After |
|---|---|
| Peers added by URL and access token | Nodes joined once by key over Tailscale |
| Groups and messages live in the host's SQLite as mutable documents | Groups and messages are hashed, signed, append-only records per node |
| Remote agents proxied through `::` IDs | The hub routes to the owning worker by node ID |
| Transfers base64 JSON, 32 MiB cap | Chunked, content addressed, resumable |
| `mace start` printing local and Wi-Fi links | Worker as a service; `mace-server start` printing the Tailscale HTTPS link and a QR code |

Existing data is migrated once: each current message becomes a record owned by the local node, chained in its current order.

## Build order

1. Records: canonical envelope, hashing, signing, hybrid logical clock, per-node chains, migration of existing data. Tests for hash stability, ordering under clock skew, and chain verification. Done: `internal/records`, schema v4 (`records`, `heads`), node key at `<state>/node.key`. Every message is also written as a record and carries its ID in `record`; earlier messages are backfilled once at start. The interface still reads messages until step 4.
2. Worker service: systemd user unit, linger, `mace setup`. Done: `mace setup [--port] [--host] [--no-linger]` and `mace setup --remove`. The unit captures the shell's PATH so agent CLIs are found, restarts on failure, and start output under systemd leaves out the token. Checked on real systemd with an isolated unit: active, restarted 7 seconds after a forced kill, all four CLIs detected, removed cleanly. Linger was not exercised.
3. Joining: node keys, `mace join`, QR code, discovery over Tailscale. Done except the QR code, which moves to step 6 with phone sign-in. `mace join --invite` prints a one-time code (10 minutes, spent atomically); `mace join <address> <code>` proves key ownership by signing the code, and the accepting machine calls back over Tailscale to confirm the key. Requests between machines are signed (`X-Mace-Node`, `X-Mace-Time`, `X-Mace-Signature` over method, path, time, body hash; 2 minute window). New machines are announced, and every 15 second check exchanges machine lists; removed machines are remembered. `mace machines` lists availability. Checked live with three isolated servers on this machine's tailnet address (100.94.153.17): join, refused reuse, introduction, unavailable within 2 seconds of a stop, back on restart. Not yet checked across two real machines. The older peer model (URL plus token, Connected hosts page) still exists until step 4 replaces it.
4. Hub: head comparison, chain walking, caches, merged views, routing actions to owners, offline gaps in the interface. Sync done: `GET /node/heads` lists every chain a node holds (own and cached), `GET /node/chain` walks one back from its head to a known record in pages of 500, and the fetching node verifies every record against the writer's key, so copies relayed by a third node are trusted only by signature. Rewritten history is refused and the local copy kept. Sync runs after every successful availability check; `POST /records/sync` runs it now. `GET /records/{conversation}` returns the merged, causally ordered view with cached flags and the offline nodes holding part of it. Checked live between two isolated servers on the tailnet. Done for the rest of step 4: a joined node is a peer whose requests are signed with its node key (no token), allowed only on the owner paths machines need (agents, turns, controls, commands, models, profiles, metrics, pause, files); agents from every node appear in one list; the owner's actions on a remote agent go to its node. A private chat with an agent merges the other machine's records of the same chat (the hub's `<node>::<id>` and the worker's `<id>`) as read-only messages marked with their machine; a `records` event reloads the open chat after a sync. A turn for an agent on an offline node waits for it, and chats show which machines are offline.
5. Chunked files and transfers. Done: `internal/blobs` stores 4 MiB chunks under their SHA-256 at `<state>/blobs`, with a manifest whose hash is the file ID. Sending to a joined node stores the file, then asks the node (`POST /transfers/fetch`) to pull the manifest and only its missing chunks (`GET /blobs/{hash}`, signed), verifying each before storing and assembling. Retries per chunk; no size cap; identical content is stored once. Hosts connected by token keep the older 32 MiB copy.
6. `mace-server start` with the Tailscale HTTPS link, home-screen install, and notifications. Done: `mace-server start|qr|status|stop` (also `mace server ...`; `make install` links `mace-server`) reads the tailnet name from `tailscale status --json`, sets `MACE_PUBLIC_URL`, runs `tailscale serve --bg --https=443`, and prints a one-time sign-in QR. Sign-in codes (`signin-code` documents, 10 minutes for QR, 2 for machine switching, spent atomically) replace the token in links. Web app manifest and service worker for home-screen install; Web Push with VAPID keys at `<state>/push-keys.json`, sent for private replies, owner mentions, and question, blocker, decision, plan, and done flairs, and shown only when no MACE window is visible. Settings lists joined machines; Open asks that machine for a one-time link (`POST /nodes/{id}/open` to `POST /node/signin`) so the phone switches already signed in.

## Context and quota

Agents should spend their quota on the owner's work, not on re-reading MACE. Three parts, all built on existing pieces:

- **Slim turns.** A resumed native session already holds the team header and the awareness block, so MACE hashes their stable parts and sends them again only when they change or the native session is new. The request, the room, and new messages are always sent. Measured with the test agents: a resumed group turn went from 4,120 to 571 bytes. Each turn's prompt size is logged as `prompt_bytes` on `TURN_STARTED`.
- **Context on demand (the context DB).** The conversation store every node already keeps is the context DB. Agents fetch what they need with `mace ctx team | plan | recent [n] | thread @handle | search <words>`, served by `GET /api/v1/ctx/{room}/{view}`. Each agent process gets `MACE_CTX_URL`, `MACE_CTX_AGENT`, `MACE_CTX_CHAT`, and a read-only token derived from its session ID with a key that lives only in the daemon's memory; it can read only conversations it belongs to. Following ToolCaching (arXiv 2601.15335), fetches are read-only and keyed by their parameters, and a repeat fetch whose result has not changed for the same native session returns a one-line note instead of the same text.
- **Code graph.** When an agent works in a git repository and graphify (`uv tool install graphifyy`) is installed, MACE builds and refreshes the repository's graph with graphify's code pass (tree-sitter, `GRAPHIFY_BACKEND=none`, so no model and no quota), at most every 30 seconds per folder, and adds `graphify-out/` to the repository's private `.git/info/exclude`. Turns then tell the agent to ask `graphify explain`, `graphify path`, or `graphify query --budget 1500` before grepping. The filesystem root and the home folder are never indexed.

Still to do: workers on other machines reach the hub's context and code graph through their own node, which forwards signed requests to the hub (with hub step 4 routing); token usage reported by the CLIs, per agent and group, with a budget that pauses a run.

## Verification that needs the real environment

- Two or more real machines on the owner's tailnet, including one going to sleep and returning.
- `tailscale serve` HTTPS on the owner's phone, home-screen install, and notifications.
- Localhost pairs of daemons prove the protocol only and are reported as such.
