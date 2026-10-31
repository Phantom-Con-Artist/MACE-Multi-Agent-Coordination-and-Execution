# MACE design: remote coworkers, live view, and always-on hosts

Status: approved direction, awaiting review of this document. Date: 2026-10-31.

This document describes the next stage of MACE. The owner controls their machines from a phone, chats with agents directly, and runs groups where agents act as autonomous coworkers. It also covers live inspection of desktop apps, one-step setup, and keeping hosts reachable.

Existing behavior that is not mentioned here stays as it is. AGENTS.md constraints still apply.

## Goals

1. Groups are live channels. The owner and every agent can post at any time. Agents work in parallel.
2. One agent is one identity. agy in a private chat and agy in a group are the same coworker, with the same memory, model, and working folder. Agents know they are part of MACE and which conversations they belong to.
3. `@` mentions are a real protocol for owner-to-agent and agent-to-agent addressing. `/` commands control an agent from its private chat. Both offer suggestions while typing.
4. Agents can post files into chat. The owner can view any file from a phone regardless of size.
5. The owner can see and operate a desktop app, such as Blender, from a phone.
6. Setup takes two commands and a QR scan. Adding another machine takes one command and a code.
7. Hosts stay awake on AC power, fall back to normal sleep on low battery, and are controllable remotely.

## Non-goals

- Exposing the CLIs' own built-in slash commands, such as Claude Code's `/compact`. Slash commands are MACE commands only.
- Waking a machine from the internet with no helper on its network. This is not physically possible on the owner's Wi-Fi setup.
- Replacing the native CLIs' own permission and sandbox models.

## Current state (summary of the review)

- A group run is a single relay: one active speaker per room, round-robin order ([collaboration.go](../internal/api/collaboration.go)). Owner messages during a run are rejected with "pause it before sending a new task".
- A session can reply in only one conversation at a time. Private and group chats share the session ID but not memory.
- Every turn starts a fresh CLI process with the last 40 messages, so agents lose their own tool context between turns.
- `@Name` only selects the next speaker by substring match on the previous reply. Names with spaces break.
- The browser polls messages every second and refetches the whole history.
- Text preview reads only the first 64 KiB. Remote transfers are base64 JSON with a 32 MiB cap.
- Desktop launch uses `gio launch` with no way to see or control the result.

## 1. Agent identity and memory

### One coworker per session

A `Session` becomes the coworker's identity on its host. New fields:

| Field | Purpose |
|---|---|
| `handle` | Single-token mention name, unique on the host. Generated from the provider (`agy`, `muse`, `claude`, `codex`), then `agy-2` and so on. Editable with `/rename`. |
| `model` | Model ID passed to the CLI with `--model`. Empty means the CLI default. |
| `native_session` | The provider's own session or conversation ID, used to resume. |
| `cursors` | Per conversation, the ID of the last message this agent has seen. |

Remote coworkers are addressed as `handle@host`, for example `muse@pc`. Within a group, a unique bare handle also works.

### Native session continuity

Each coworker keeps one native CLI session for all of its conversations, so it remembers its own tool calls, files read, and reasoning.

| Provider | First turn | Later turns | Session ID source |
|---|---|---|---|
| Claude Code | `claude -p --output-format json --session-id <uuid>` | `--resume <uuid>` | Chosen by MACE |
| Codex | `codex exec --json` | `codex exec resume <id> --json` | `thread.started` event |
| Muse | `muse exec --json` | `muse resume <id>` | JSONL session event (to confirm) |
| Antigravity | `agy --print` | `--conversation <id>` | To confirm; fall back to transcript replay |

Phase 1 starts with a short spike that confirms each row against the installed CLIs. An adapter that cannot resume reliably falls back to today's transcript replay. That fallback is shown in the agent's info panel, not hidden.

Because the native session holds memory, each turn sends only what is new. The turn contains a MACE awareness header plus the messages after the agent's cursor in the conversation that triggered the turn.

### MACE awareness header

Every turn tells the agent:

- That it is a MACE coworker, its handle, host, model, and working folder.
- Every conversation it belongs to: its private chat with the owner and each group with members, roles, and mode.
- Which conversation this turn is for, and that its reply is posted there.
- A one-line digest of recent activity in its other conversations.
- The output conventions in section 2.

`/new` starts a fresh native session. Since the identity is shared, this resets the agent's memory everywhere. The command says so before it runs.

### Per-agent inbox

A native session can only run one turn at a time. Each coworker therefore has an inbox worker that runs turns in order. Different coworkers run in parallel.

- A turn request names a conversation and the message that triggered it.
- Pending requests for the same conversation merge into one, since the agent reads everything after its cursor anyway.
- Owner messages in the private chat go to the front of the queue. Continuous-mode chatter goes to the back.
- Stop cancels the current turn and clears the inbox. Pause on a group removes that group's requests from every inbox.
- Remote coworkers keep their inbox on their own host. The existing `/sessions/{id}/turn` endpoint enqueues there and waits.

This replaces the per-room relay loop and the "already replying in another conversation" error.

## 2. Conversation protocol v3

### Who responds

| Message from | Mentions | Who gets a turn |
|---|---|---|
| Owner | none | Every available agent in the conversation, in parallel |
| Owner | `@agy @muse` | Only the mentioned agents |
| Owner | `@all` | Every available agent |
| Agent | none | Nobody |
| Agent | `@muse` | Only the mentioned agents |

In a private chat the single agent always responds to the owner.

Owner messages are never rejected because a conversation is busy. An agent mid-turn sees the new message on its next turn.

### Modes

- **Reply limit** counts total agent replies in the conversation, as today.
- **Goal** works as today and stops when an agent reports `[GOAL_COMPLETE]` or the budget is reached.
- **Continuous**: after any agent posts, every other agent gets a low-priority turn, at most one every 20 seconds per agent. It runs until paused or a turn fails.
- **Hop limit**: in every mode, a chain of agent-triggered turns started by one owner message stops after 8 hops. The limit is configurable and the conversation shows when it is reached.

### Agent output conventions

Agents are told these in the awareness header. MACE parses them from the reply.

| Convention | Meaning |
|---|---|
| `@handle` | Hand a turn to that coworker |
| `[PASS]` | Nothing useful to add. The reply is not posted. This keeps "everyone answers" from becoming noise. |
| `[GOAL_COMPLETE]` | Goal-mode completion claim, as today |
| `[[file: /absolute/path]]` | Attach a host file to the message (section 4) |

### Message shape

`Message` gains `mentions` (resolved session IDs), `files` (a list replacing the single `file` field, which is still read), `kind` (`chat`, `command`, or `system`), and `hop`. Mentions are resolved on the server, so the UI and agents agree on who was addressed.

### Realtime delivery

A single authenticated stream, `GET /api/v1/stream` using server-sent events, carries message, run, session, inbox, and host events. It is resumable with `Last-Event-ID`. The browser drops the one-second polls. It falls back to polling only while the stream is disconnected, and says so in the status bar.

Message history becomes paginated: `GET /chats/{room}/messages?before=<id>&limit=50`. Storage gets an indexed messages table instead of loading every document of a kind.

## 3. Composer: `@` and `/` suggestions

Both use one accessible combobox above the composer. It supports arrow keys, Enter or Tab to accept, Escape to close, and tapping on phones. It works with IME composition and the existing drafts.

- Typing `@` lists the conversation's members by handle, name, provider, and state (ready, replying, stopped), plus `@all` in groups. Accepting inserts `@handle `.
- Typing `/` at the start of a message in a private chat lists commands with one-line descriptions. Commands are not offered in groups.

| Command | Effect |
|---|---|
| `/model` | Lists models and sets the agent's model. It applies everywhere the agent is. |
| `/models` | Shows the available models and the current one |
| `/cd <path>` | Changes the working folder, with folder suggestions as you type |
| `/new` | Starts a fresh native session after confirmation |
| `/stop` | Stops the current turn and clears the inbox |
| `/status` | Shows the model, folder, inbox, groups, and native session state |
| `/rename <handle>` | Changes the mention handle |
| `/files` | Opens the host file browser at the agent's folder |
| `/help` | Lists commands |

Model lists come from the CLI where it offers one, such as `agy models`. Other providers get a curated list plus free entry, validated on the next turn. Command results appear in the chat as compact system notes and are not sent to the agent. The agent learns about a model or folder change from its next awareness header.

## 4. Files

### Agents posting files

`[[file: /path]]` in an agent reply attaches the file after MACE checks that it exists inside `MACE_FILE_ROOT`. Missing files become a visible note on the message, never a silent drop. The owner opens attachments in the viewer below.

### Viewing any size

All viewing streams with HTTP range requests, so file size does not matter.

| Type | Viewer |
|---|---|
| Images | Native `img`, tap to zoom |
| Video and audio | Native players with range seeking (already works) |
| PDF | pdf.js, loaded on demand, page by page. Mobile browsers do not render PDFs inline reliably. |
| Text, logs, code | Virtualized viewer that reads 256 KiB windows on demand, jumps by byte offset, and follows the end of growing logs. Syntax highlighting only for windows under a size cap. |
| Other binaries | Paged hex view, plus download |

A server endpoint returns line-aligned windows and searches a file without sending all of it to the phone.

### Transfers between hosts

Base64 JSON is replaced with a streaming upload. The sender hashes with SHA-256 while streaming, sends the digest as a trailer, and the receiver verifies before renaming into the inbox. Transfers resume from the last confirmed offset. The 32 MiB cap is removed. The receiver checks free disk space first.

## 5. Live View

Live View lets the owner see and operate a desktop app on a host from the phone. The target is KDE Plasma on Wayland, which is what the owner runs.

### Pipeline

1. Opening an app from Host, or tapping Live View on a host, asks the desktop portal (`xdg-desktop-portal-kde`) for a ScreenCast session and a RemoteDesktop session.
2. The portal returns a PipeWire stream. GStreamer, already installed, encodes it with hardware encoding when available and software VP8 or H.264 otherwise.
3. Video goes to the browser over WebRTC (pion in Go). Signaling uses the existing authenticated connection. Tailnet and home Wi-Fi peers connect directly, so no relay server is needed.
4. Touch and keyboard input return over a WebRTC data channel. MACE forwards them through the RemoteDesktop portal as real pointer, scroll, and key events.

### Phone controls

- Tap clicks, long-press right-clicks, one-finger drag moves with the button held, two-finger drag scrolls, and pinch zooms the view locally.
- A key bar offers Shift, Ctrl, Alt, Super, Esc, Tab, arrow keys, the on-screen keyboard, and a middle-mouse toggle for Blender-style navigation.
- A crop control frames the launched app's window on small screens.

### Consent and limits

- KDE asks once on the host to allow screen sharing and control. MACE stores the portal's restore token so later sessions start without a prompt. This needs real verification on the owner's machine.
- Phase 6 captures the whole monitor because window capture identity does not survive app restarts. Cropping to the app window is done on the phone.
- A locked screen captures as the lock screen. Section 7 covers unlocking on demand.
- Later phase: launch apps on a KWin virtual output that keeps running behind the lock.

## 6. Setup

### First machine

```sh
curl -fsSL <release-url>/install.sh | sh
mace setup
```

- The web UI is embedded in the binary with `go:embed`, so installation is one file in `~/.local/bin`.
- `mace setup` installs a systemd user service with restart on failure and enables linger, so MACE runs from boot without a login.
- It detects installed agents and reports which need a provider login.
- If Tailscale is installed, it configures `tailscale serve` to give MACE an HTTPS tailnet address. If the operator permission is missing, it prints the one `sudo tailscale set --operator=$USER` command needed. HTTPS enables installing MACE on the phone home screen and web push notifications. Without Tailscale, MACE falls back to the embedded `tsnet` path or the LAN address.
- It prints a QR code of the sign-in link. Scanning it signs the phone in.

Every step has a default. Re-running `mace setup` is safe and repairs a broken install.

### More machines

`mace join` on the new machine finds MACE hosts on the tailnet and LAN and asks for the 6-digit pairing code shown in the phone's Hosts screen. Codes last 10 minutes, are single-use, and are rate-limited. The hosts then exchange per-peer tokens that can be revoked on their own, instead of sharing the master token.

### Phone app

A web app manifest and a small service worker make MACE installable. Notifications use Web Push for agent mentions, goal completion, failures, and low battery. Push payloads are encrypted and contain no message text unless the owner enables previews.

## 7. Power and reachability

### Staying awake

MACE holds a logind inhibitor (`sleep:idle:handle-lid-switch`, block mode) according to a power policy. Battery state comes from UPower or `/sys/class/power_supply`.

| State | Behavior |
|---|---|
| AC power | Inhibit idle sleep and lid-switch sleep |
| Battery above the threshold (default 25%) | Keep inhibiting and notify the owner once |
| Battery at or below the threshold | Release the inhibitor, notify the owner, and let the machine sleep normally |

Inhibitors are released automatically by the system if MACE exits, so a crash cannot pin a machine awake. The display may still turn off. Live View wakes it.

### Lock screen

The screen still locks for physical security. When the owner opens Live View, MACE unlocks the session with `loginctl unlock-session`, which KDE's locker honors. It locks again when Live View closes or after an idle timeout. This must be verified on the owner's machine before it is relied on.

### Power panel

Each host shows battery and AC state, the Stay awake switch, and Lock, Unlock, Sleep, Restart, and Shut down. Restart and Shut down require confirmation and are audited.

### Presence

Hosts report heartbeats every 30 seconds with battery and AC state. The phone shows each host as online, asleep (last seen time and battery), or unreachable.

### Wake (optional)

Wake-on-LAN is optional and hidden unless a host reports a wired interface with wake support (checked with `ethtool`) and an online MACE host shares its network. The owner's laptop and PC are on home Wi-Fi, so this does not apply to them. `mace doctor` mentions the BIOS "restore on AC power loss" setting for machines that should come back after a power cut.

## 8. Robustness, safety, and recovery

This section comes from a review of the current code for failure paths. Section 8.1 lists defects that exist today and are fixed in phase 0, before new features add load to them. Sections 8.2 to 8.7 are rules every later phase must follow.

### 8.1 Defects found in the current code

| # | Defect | Where | Consequence | Fix |
|---|---|---|---|---|
| D1 | The master access token is inherited by every child process. maced receives `MACE_AUTH_SECRET` in its environment and the supervisor launches children with `os.Environ()`. | [main.go](../cmd/mace/main.go) start, [supervisor.go](../internal/agent/supervisor.go) `StartAgent` | Every agent CLI, every shell command an agent runs, and every app launched from Host (Blender included) can read the token and fully control MACE and connected hosts. A prompt injection in a web page an agent reads is enough. | Unset `MACE_AUTH_SECRET` and `MACE_TS_AUTHKEY` in maced after reading them. Children get an allowlisted environment with all `MACE_*` secrets removed. Pass the token to maced through a 0600 file, not the environment. Add a test that a launched child cannot see it. |
| D2 | A stdout line over 1 MiB stops the reader. `bufio.Scanner` returns `ErrTooLong`, the error is ignored, and the pipe is no longer drained. | `pipeReader` | The child blocks on a full pipe until the 10 minute timeout. Claude Code's JSON output is one line, so one long reply hangs the turn and then fails it. | Read with `bufio.Reader` and split oversized lines into continuation events. Always drain to EOF. Record reader errors as events. |
| D3 | The process pool fails immediately when full. | `StartAgent` returns `ErrPoolCapacityFull` | With parallel groups, the 11th concurrent turn fails instead of waiting. | Inbox workers wait for a slot with their context. The UI shows "queued, waiting for a free slot". |
| D4 | Agent processes survive a daemon crash. Children run in their own process group (`Setpgid`), and nothing ties their lifetime to maced. | `proc_unix.go` | After a crash or `kill -9`, orphaned CLIs keep running and may keep editing files with no supervisor. | Set `Pdeathsig: SIGKILL` on Linux. Under systemd, also rely on `KillMode=control-group`. On startup, find leftover processes labelled by MACE and stop them. |
| D5 | A turn has a fixed 10 minute wall-clock limit. | `turn` `TimeoutSeconds: 600` | Long, legitimate agent work is killed while it is still making progress. | Use an inactivity timeout (no output for a configurable time, default 10 minutes) plus a configurable hard cap (default 2 hours). |
| D6 | Replies are assembled from at most 10,000 events. | `GetAgentEvents(..., 10000)` | Long outputs are silently truncated. | Page through all events, or accumulate the reply while streaming. |
| D7 | `agent_events` and `audit_log` grow forever. Each stdout line is a separate synchronous insert on the single connection. | storage | The disk fills over weeks. Parallel agents contend for the one writer. | Batch event inserts. Add retention: keep 14 days of native output by default and keep messages and audit forever. Run an hourly checkpoint. |
| D8 | The schema has no version. Migration is `CREATE TABLE IF NOT EXISTS` only. | `migrate` | Schema changes in later phases cannot be applied or rolled back safely. | Add `PRAGMA user_version`, ordered migrations, and a `VACUUM INTO` backup before each migration. |
| D9 | Message sends are not idempotent. | `messages` POST | A phone on a weak network retries and posts twice, which starts agent work twice. | The client sends a `client_id`. The server returns the existing message for a repeated ID. |
| D10 | Agents are treated as available when a connected host has never reported them. | `available` | A group waits on, then fails against, an agent that does not exist. | Unknown remote agents count as unavailable until the peer confirms them. |
| D11 | The session cookie is a fixed hash of the token and expires only in the browser. | `server.go` | A copied cookie works until the token changes. No phone can be signed out by itself. | Per-device sessions stored on the server, with a device list, last-seen time, and revoke. Rotating the token signs out every device. |
| D12 | The global 60 second request timeout applies to everything except `/turn`. | `server.go` | The planned event stream, large uploads, and Live View signaling would be cut off. | Exempt streaming routes explicitly and give them their own idle limits. |

### 8.2 Owner presence guard

Requested on 2026-10-31: when the owner's connection goes dark during work, agents stop, and the owner can continue where they left off.

- Signed-in browsers ping every 5 seconds. A device counts as gone after 10 seconds of silence. After a further grace period (default 15 seconds, `MACE_PRESENCE_GRACE_SECONDS`, `0` for as soon as possible, `off` to disable), the host stops all agent processes it started and marks their conversations Interrupted with a visible note.
- The grace period exists because phone browsers suspend pages when the screen locks or the app is backgrounded. With no grace, agents would stop every time the screen dims.
- A closing page reports departure at once, keeping 3 seconds of slack so a reload does not count.
- The guard is armed only after a device has checked in, so work started only through the API is unaffected. It disarms after it fires.
- Turns requested by a connected host are left to that host. Its own guard, or the dropped connection, ends them. Short TCP keepalives notice a vanished host in about 25 seconds.
- Desktop apps launched from Host are never stopped.
- Continue resumes each conversation. Until phase 1 adds native session resume, the agent relies on the transcript and on checking what was already done.

### 8.3 Failure model

| Failure | Detection | Behavior |
|---|---|---|
| maced crashes | systemd | Restarts after 2 seconds, with a crash-loop limit of 5 starts in 2 minutes. After that, it stays down and `mace status` explains why. |
| Host reboots | systemd user service with linger | MACE is back before login. Agents work. Live View waits for a desktop session. |
| A turn is in progress during a crash | On startup, the inbox entry is still marked running | The agent message shows "interrupted, may have partially run". It is never retried automatically, because the agent may already have changed files. The owner can tap Retry. |
| Queued inbox requests during a crash | Inbox entries are persisted | They are kept but held. Groups show "paused after restart" with a Resume button, matching the current rule that work never silently resumes. |
| A native session cannot be resumed (file deleted, CLI upgraded) | Resume fails with a recognizable error | Start a fresh native session, replay the transcript window, and post a system note that the agent's memory was reset. |
| A provider login expires or hits a rate limit | Error text matched per adapter | The agent's state becomes "needs login" or "rate limited, retry after X". The rest of the group continues. The owner is notified once, not on every turn. |
| A connected host goes offline mid-turn | Request error or missed heartbeats | Its agents show as unreachable. The turn fails visibly for that agent only. Other agents continue. |
| Tailscale is down | `tailscale status` and failed requests | The phone tries the LAN address, which the installed app remembers. Peers are addressed by MagicDNS name with the last LAN IP as fallback. |
| The disk is full | SQLite write errors and a free-space check | New turns are refused with a clear banner. Reading history still works. Retention runs early. |
| Database corruption | `PRAGMA quick_check` at startup | Move the bad file aside, restore the newest daily `VACUUM INTO` backup, and report what time range was lost. |
| The phone loses connection | Event stream drops | The app shows "Reconnecting" and keeps drafts and unsent messages. It resumes with `Last-Event-ID`, or refetches if the gap is too old. |
| The battery reading fails | UPower and sysfs both unreadable | Release the inhibitor. A failure must lead to normal sleep, never to an unbounded awake state. |

### 8.4 Runaway and cost safety

- **Pause everything.** One button on the phone and `mace pause-all` stop every running turn, hold every inbox, and pause every group on this host and on connected hosts. It is always visible while any agent is working.
- **Limits.** A global concurrency cap (default 6 turns), at most one continuous-mode turn per agent every 20 seconds, the hop limit from section 2, and an optional daily turn budget per agent. When a limit is reached, the conversation says so; nothing stops silently.
- **Loop detection.** If two agents exchange near-identical messages three times, the group pauses with a note.
- **Agent capability boundaries.** Agents keep their native CLI permissions, but MACE adds nothing on top. They never receive MACE credentials (D1), cannot post as another member, and cannot call host actions such as launching apps or Live View.

### 8.5 File safety

- Every path is resolved through `os.Root` against `MACE_FILE_ROOT`, so symlinks cannot escape it.
- Agent attachments that would be copied to another host need one tap of owner approval if they match a sensitive pattern: `~/.ssh`, `~/.gnupg`, `~/.config/mace`, browser profiles, `.env` files, and key files. Same-host viewing is not restricted, because only the owner can see it.
- Transfers write to a temporary file, verify SHA-256, then rename atomically. An interrupted transfer never leaves a partial file under the final name.

### 8.6 Power and Live View safety

- Shut down, Restart, and Sleep require confirmation, and a Live View warning shows when the action would end the stream.
- When Live View disconnects for any reason, MACE releases every held key and mouse button. A dropped connection must not leave Ctrl held on the host.
- Live View closes after 10 minutes without input from the phone. The screen locks again if MACE unlocked it.
- When the KDE session restarts, the restore token is no longer valid. Live View then asks for consent again on the host instead of failing silently.
- After a reboot with no desktop login, Live View reports "No desktop session. Log in on the host or enable automatic login." It never claims to work.
- Lid-closed operation is reported in the Power panel along with temperature where available, so the owner can see a hot laptop.

### 8.7 Updates

`mace update` downloads the new binary next to the old one and verifies its checksum. It backs up the database, swaps the binary atomically, and restarts. If the health check fails within 30 seconds, it swaps back and restarts the previous version.

## Phases

Each phase ends with passing tests and a browser check. Fixture and localhost results are reported as such and never as live verification (see AGENTS.md).

| Phase | Scope | Depends on |
|---|---|---|
| 0 | Fix D1 to D12, persisted inbox model, schema versioning and backups, Pause everything, failure-path tests (crash mid-turn, oversized output, full pool, orphan cleanup, duplicate send) | none |
| 1 | Adapter spike, coworker identity, native resume, per-agent inbox, protocol v3 dispatch, `[PASS]`, hop limit, SSE stream, paginated history | 0 |
| 2 | `@` and `/` suggestions, private-chat commands, `/model` everywhere | 1 |
| 3 | Agent file attachments, streaming viewer, pdf.js, hex view, streaming resumable transfers | 1 |
| 4 | Embedded UI, installer, `mace setup`, Tailscale serve, QR sign-in, `mace join`, per-peer tokens, per-device sessions, PWA and push, `mace update` with rollback | 0 |
| 5 | Power policy, lock and unlock, power panel, presence, optional wake | 4 |
| 6 | Live View over portal, PipeWire, GStreamer, and WebRTC | 5 for unlock |

Phases 3 and 4 can run in parallel with phase 2.

## Verification that needs the real environment

These cannot be proven with the diagnostic provider or two localhost daemons:

- Native resume and model switching for each installed CLI with a live provider login.
- Parallel group turns with real agents, including `[PASS]` and agent-to-agent mentions.
- `tailscale serve` HTTPS, phone installation, and Web Push on the owner's phone.
- `mace join` between the laptop and the PC over home Wi-Fi.
- logind inhibition, lid-close behavior, low-battery release, and `loginctl unlock-session` with KDE's locker.
- Live View consent, restore tokens, latency, and Blender input on the owner's desktop.

## Phase 0 status (2026-10-31)

Done on branch `phase-0-hardening`, with tests: D1 to D6, D8 to D12, retention and daily backups from D7, the presence guard, Pause everything (UI and `mace pause-all`), Interrupted state with Continue, and the signed-in devices list.

Moved to phase 1, where the code they depend on is written: batched event inserts (rest of D7), the persisted per-agent inbox, and loop detection.

## Open questions

- Antigravity conversation ID discovery in print mode. Resolved by the phase 1 spike.
- Whether the continuous-mode interval and hop limit defaults feel right in practice. Both are configurable.
