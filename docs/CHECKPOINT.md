# MACE checkpoint, 2026-10-31 (third session)

Branch: `phase-0-hardening` (not merged to main). All Go tests pass; the frontend type-checks and builds. Working tree clean after the last commit.

## Done (commits on the branch, oldest first)

1. Phase 0 hardening: token never reaches child processes, oversized output drained, slot waiting, orphan reaping, idle and hard turn limits, schema versions with backups and corruption restore, idempotent sends, per-device sign-in, presence guard (agents stop when every device goes quiet), Pause everything, Interrupted plus Continue.
2. Phase 1: per-agent inboxes, parallel dispatch, protocol v3 (@mentions, [PASS], hop limit 8, loop detection, per-message budgets with generations), one native session per agent shared across chats (agy --conversation, muse/claude --session-id, codex exec resume), live tool steps, SSE event stream, paged messages table.
3. Brand: exact logo SVGs, Archivo, palettes Signal, Ink, Ember, Carbon, Auto (`web/src/theme.css`, `theme.ts`), messenger polish (tile avatars, unread, receipts from read cursors, @ menu, date dividers, triangle tails), decluttered UI.
4. Agent profiles: name, handle, role (follows into groups unless a group sets one), color profiles, official marks for Claude Code and Muse (Simple Icons, CC0), owner logos in `<state>/avatars/`.
5. Public site rebuilt with original vector art (hyperbolic tiling, `scripts/hyperbolic-art.py`); raster art removed.
6. B3: per-machine monitor (CPU, memory, disk, load, GPU via nvidia-smi or AMD sysfs) at `/api/v1/metrics`, streamed as `metrics` events, peer proxy `/peers/{id}/metrics`; SVG gauges, sparkline, activity bar chart; compact agent rows on Host.
7. Phone review (`4242ca6`): Activity as a timeline (sentence-case events, MACE instead of internal actors, a mark per event kind, details as a wrapped list), truthful Host empty state, CPU peak, a real wallpaper switch, full-width device rows, MACE in product copy.
8. Phase 2 commands (`67c37e3`): `/model`, `/models`, `/cd`, `/new`, `/stop`, `/status`, `/rename`, `/files`, `/help` in private chats via `POST /api/v1/sessions/{id}/command`. Results are `kind: "note"` messages kept out of the agent's transcript. Models from `agy models` (cached 10 min), Claude Code aliases, or free entry (`GET /sessions/{id}/models`). Composer suggests commands, models, and folders; `/new` confirms in a dialog. Verified live with Muse.
9. Host apps as a launcher grid (`f9f55ed`), grouped by desktop category, monogram tiles in the avatar color profiles (`category` on `/api/v1/apps`).
10. SIGHUP fix (`b32cf93`): maced logs and ignores hangups, so closing the terminal that ran `mace start` no longer kills it silently; `mace status` reports an unexpected exit when a PID record is left behind.
11. Hub and worker architecture recorded (`6c79195`): docs/DESIGN-HUB-WORKER.md.
12. Hub and worker step 1, records (`c4e940b`): `internal/records` (SHA-256 of a canonical v1 envelope, Ed25519 signature, per-node chain via `prev`, `seen` heads, hybrid logical clock, causal `Order`, `VerifyChain`), schema v4 (`records`, `heads`), node key at `<state>/node.key` (0600). Every message is also written as a record and carries its ID in `record`; earlier messages backfill once at start. Checked on a copy of the owner's database: 20 messages, 6 conversations, backup taken, no duplicates on restart.
13. Hub and worker step 2, worker service (`0a7e642`): `mace setup [--port] [--host] [--no-linger]`, `mace setup --remove`. User unit captures PATH, `Restart=on-failure`, token kept out of the journal under systemd. Checked on real systemd with an isolated unit (`MACE_SERVICE_NAME=mace-test`): restarted 7 s after a forced kill, all four CLIs detected, removed cleanly.

14. Hub and worker step 3, joining: `mace join --invite`, `mace join <address> <code>`, `mace machines`; signed node requests, introductions, availability every 15 s (`internal/api/nodes.go`, `cmd/mace/nodes.go`). Checked live with three isolated servers on the tailnet address 100.94.153.17. QR moved to step 6.

15. Hub and worker step 4, first slice, record sync (`internal/api/sync.go`): heads and chain endpoints, verified incremental fetch, cached copies relayed through other nodes, merged view at `GET /api/v1/records/{conversation}` with offline nodes, sync after each availability check. Checked live between two isolated servers on the tailnet.

16. Team behavior (`4de81f4`, `a667266`, `232788c`, `5cfb0d7`, `da47158`): owner messages preempt addressed agents in groups; no autostart; until-done mode ending on [done]; short chat-style turns with casual or professional tone; request, goal, and roles in every group turn; self-chosen roles with [role: ...]; flairs with custom ones; [share: path] and auto-files for long pastes with tagged-only paths; side chats with [dm @handle] and [group]; presence grace 10 minutes plus Keep working while I'm away; Muse model calls kept apart; quoted handles ignored. Live test with agy and Muse in ~/Development/mace-playground: two tasks done with correct code and passing tests, replies 65 to 303 characters, a mid-run change of plan cancelled the old work with no stale replies.

## Not yet verified

- Claude Code and Codex adapters live (agy and muse were verified live), including `/model` on them.
- Linger (start at boot before login); not run because it changes a system setting.
- Joining and availability across two real machines (the tailnet has only `fedora` so far).
- Anything across real machines: peer monitors, remote commands, Tailnet, phone hardware, two-machine Wi-Fi.
- Side chats and [share] with real agents (covered by tests; the live tasks were too small to need them).
- Muse's shell fails when its folder is `/` or the home folder: the system bwrap lacks --ro-bind-symlink. It works in project folders. Not a MACE fix.
- Launching an app from the grid (not clicked, to avoid opening windows on the owner's desktop).

## Next, when this work resumes

Follow the build order in docs/DESIGN-HUB-WORKER.md:

4. Hub, rest of the step: the interface reads `GET /records/{conversation}` instead of messages, agents from every node in one list, the owner's actions routed to the owning node (node-signed, which widens node auth beyond `/api/v1/node/`), offline gaps shown in chats. Sync is done.
5. Chunked, content-addressed files and transfers (absorbs the transfer part of phase 3).
6. `mace-server start` with the Tailscale HTTPS link, QR sign-in, home-screen install, notifications, and a hub switcher on the phone.

After that: the rest of phase 3 (streaming viewer, pdf.js, hex view), the animated CLI (B5), power (phase 5), Live View (phase 6).

## Owner's environment

- Installed `mace` and `maced` in `~/.local/bin` date from 00:11 on 2026-10-31 and predate the SIGHUP fix and everything after. `make install` then `mace setup` switches the owner to this build; not done, the owner has not asked.
- The owner's server on port 8080 was stopped at the time of writing (it had died from the SIGHUP bug).
- Firewall zone FedoraWorkstation allows TCP 1025 to 65535 on Wi-Fi (`wlp2s0`, 192.168.0.6). `rain0` (10.77.0.1) is a VPN.

## Be careful

- Never run `bin/mace start|restart|stop|setup` without MACE_HOME, MACE_PID_FILE, MACE_LOG_FILE pointing at a scratch folder (and MACE_SERVICE_NAME for setup); otherwise it acts on the owner's live install.
- Isolated test daemons used ports 18091 to 18093 with state in the session scratchpad (gone next session).
- Tailwind scans class names: avoid utility names such as `ring`, `border`, `shadow` as custom classes.
- Go nil slices encode as JSON null; return empty slices to the browser.
- Browsers cache `index.html`; add a `?v=N` query when reloading after a frontend build.
- Audit timestamps are Go strings, not ISO; parse with the helper in `web/src/charts.tsx`.
- Records are immutable: never put local, changing state (delivery, seq) in a record body. Changing the envelope or encoding needs a new record version.
- No em dashes in product copy (AGENTS.md). Product name: MACE, Multi Agent Coordination and Execution.
- Live provider calls: agy and muse are free for testing; avoid spending Claude Code or Codex quota.
- The owner wants minimal visible controls, a messenger-grade feel, vector art only, and decisions explained briefly.
