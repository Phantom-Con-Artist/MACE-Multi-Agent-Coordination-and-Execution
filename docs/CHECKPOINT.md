# MACE checkpoint, 2026-10-31 (updated after the second session)

Saved when quota ran low. Branch: `phase-0-hardening` (not merged to main). All Go tests pass; the frontend type-checks and builds.

## Done (commits on the branch, oldest first)

1. Phase 0 hardening: token never reaches child processes, oversized output drained, slot waiting, orphan reaping, idle and hard turn limits, schema versions with backups and corruption restore, idempotent sends, per-device sign-in, presence guard (agents stop when every device goes quiet), Pause everything, Interrupted plus Continue.
2. Phase 1: per-agent inboxes, parallel dispatch, protocol v3 (@mentions, [PASS], hop limit 8, loop detection, per-message budgets with generations), one native session per agent shared across chats (agy --conversation, muse/claude --session-id, codex exec resume), live tool steps, SSE event stream, paged messages table.
3. Brand: exact logo SVGs, Archivo, palettes Signal, Ink, Ember, Carbon, Auto (`web/src/theme.css`, `theme.ts`), messenger polish (tile avatars, unread, receipts from read cursors, @ menu, date dividers, triangle tails), decluttered UI.
4. Agent profiles: name, handle, role (follows into groups unless a group sets one), color profiles, official marks for Claude Code and Muse (Simple Icons, CC0), owner logos in `<state>/avatars/`.
5. Public site rebuilt with original vector art (hyperbolic tiling, `scripts/hyperbolic-art.py`); raster art removed.
6. B3: per-machine monitor (CPU, memory, disk, load, GPU via nvidia-smi or AMD sysfs) at `/api/v1/metrics`, streamed as `metrics` events, peer proxy `/peers/{id}/metrics`; SVG gauges, sparkline, activity bar chart; compact agent rows on Host.

7. Phone review: Activity as a timeline (sentence-case events, MACE instead of internal actors, a mark per event kind, details as a wrapped list), truthful Host empty state, CPU peak, a real wallpaper switch, full-width device rows, MACE in product copy.
8. Phase 2 commands: `/model`, `/models`, `/cd`, `/new`, `/stop`, `/status`, `/rename`, `/files`, `/help` in private chats via `POST /api/v1/sessions/{id}/command`. Results are `kind: "note"` messages that never reach the agent's transcript. Models from `agy models` (cached 10 min), Claude Code aliases, or free entry (`GET /sessions/{id}/models`). Composer suggests commands, models, and folders; `/new` confirms in a dialog. Verified live with Muse: `/status`, `/cd`, `/new`, and the next turn reporting the new folder.
9. Host apps as a launcher grid grouped by desktop category, with monogram tiles in the avatar color profiles (`category` on `/api/v1/apps`).

## Not yet verified

- Claude Code and Codex adapters live (agy and muse were verified live).
- Peer machine monitors with a real second host. Phone hardware, Tailnet, real two-machine Wi-Fi.
- Launching an app from the new grid (not clicked, to avoid opening windows on the owner's desktop).
- `/model` with a live Claude Code or Codex turn, and commands on a remote agent through a real second host.

## Architecture decision (2026-10-31)

Hub and worker nodes, every node stores its own data, records named by content hash with per-node chains. See docs/DESIGN-HUB-WORKER.md. Its build order comes before the remaining phase 3 to 6 items, which it partly absorbs.

## Next, in order (see docs/DESIGN-REMOTE-COWORKERS.md, "Status and remaining work")

1. Files: streaming viewer for any size, pdf.js, hex view, transfers without the 32 MiB cap.
2. Full animated CLI (B5), then setup (phase 4), power (phase 5), Live View (phase 6).

## Be careful

- Never run `bin/mace start|restart|stop` without MACE_HOME, MACE_PID_FILE, MACE_LOG_FILE pointing at a scratch folder; otherwise it acts on the owner's live server on port 8080. The owner's server is still running an early phase 0 build from a mistaken restart; the owner was told and has not asked to change it.
- Isolated test daemon: port 18091, state in the session scratchpad (gone next session).
- Tailwind scans class names: avoid utility names such as `ring`, `border`, `shadow` as custom classes (this caused boxed gauges).
- Go nil slices encode as JSON null; return empty slices to the browser.
- Browsers cache `index.html`; add a `?v=N` query when reloading after a frontend build.
- Audit timestamps are Go strings, not ISO; parse with the helper in `web/src/charts.tsx`.
- No em dashes in product copy (AGENTS.md). Product name: MACE, Multi Agent Coordination and Execution.
- Live provider calls: agy and muse are free for testing; avoid spending Claude Code or Codex quota.
- The owner wants minimal visible controls, a messenger-grade feel, vector art only, and decisions explained briefly.
