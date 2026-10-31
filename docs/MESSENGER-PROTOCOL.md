# Mace collaboration protocol v3

The host is a single-owner trust boundary. Protected endpoints accept the printed Bearer token. POST /api/session exchanges a token for an HttpOnly, SameSite=Strict session cookie. Startup sign-in links put the token in the URL fragment; the browser exchanges it and removes the fragment. Tokens are generated and persisted with mode 0600 unless explicitly configured.

## Agents and host actions

GET /api/v1/catalog discovers supported native CLIs on PATH. POST /sessions accepts provider and optional working_dir; the default is /. GET /sessions returns logical agent instances. Each conversational turn runs a supervised native headless CLI process with that instance's working directory and recent shared transcript. Codex JSONL and Claude JSON are normalized into replies; Antigravity and Muse use their text output modes. Provider credentials and execution permissions remain native to the installed CLI.

POST /sessions/{id}/control accepts action stop, terminate, resume, or remove. Stop cancels relevant runs and processes; terminate marks the instance unavailable; resume makes it ready; remove hides it and removes group memberships while retaining history. Remote composite IDs are supported. POST /rooms/{room}/members adds a member; POST /rooms/{room}/members/{member}/remove pauses the affected run and removes membership without terminating the instance.

GET /apps discovers desktop launchers. POST /host/open accepts an app ID or filesystem path and launches gio/xdg-open under supervision. It opens on the host desktop, not inside a streamed desktop. Launch results and errors are available through native process history and logs. GET /files?path=/ browses permitted files using the daemon user's OS permissions. Files support range requests and native browser media; text previews request the first 64 KiB. MACE_FILE_ROOT can restrict browsing and transfers.

## Conversations

POST /rooms creates a group with name and member IDs. GET/POST /chats/{room}/messages reads or sends messages. A direct conversation uses the logical session ID as its room. sender defaults to you; trusted adapters can specify a member ID. Agent IDs are routing labels, not separately authenticated identities. Text can carry structured JSON data, discussion, or task handoffs. Local file attachments are passed as host paths.

GET/POST /chats/{room}/policy configures mode, limit, and goal. reply mode counts total agent responses across members; its default gives each member one turn. goal mode adds the stated goal to context and stops on an agent's [GOAL_COMPLETE] report or the response budget. The completion marker is a self-report, not independent proof. continuous mode (shown as Until done) keeps agents working on the owner's latest request until an agent replies with the [done] flair, every agent passes, a reply fails, or the owner steps in. tone is casual (default) or professional. POST /chats/{room}/pause cancels the active run and its native process group. POST /chats/{room}/start is refused: agents never begin a conversation on their own.

GET /runs returns persisted progress, reply counts, completion, failure, or pause status. Runs do not resume automatically after a restart. The orchestrator supplies recent messages (up to 40 and 100,000 bytes) to each turn and records normalized replies in the conversation. Native output remains separately inspectable in Activity. Current history presentation polls persisted state.

## Coworkers and conversations (v3)

Each agent session is one coworker with a mention handle (agy, agy-2, muse) and one native CLI session shared by its private chat and every group it joins. Turns resume that session: Antigravity with --conversation, Muse and Claude Code with a session ID Mace chooses, Codex with exec resume. When the CLI starts a new session instead (agy does this silently for an unknown ID), the conversation receives a note that the agent's memory was reset. A fresh session receives the full instructions and recent transcript; a resumed one receives only messages since its read cursor. Every turn carries an awareness block listing the agent's conversations and their latest message.

Each coworker has an inbox and runs one turn at a time; different coworkers work in parallel. Private chat messages run before group work. Who responds: an owner message without mentions wakes every available member; @handle wakes only those members; @all wakes everyone. An agent message wakes only the members it mentions. In until-done mode every other member also gets a paced turn (MACE_CONTINUOUS_GAP_SECONDS, default 20) and rounds continue until [done], pause, a failed reply, or every agent replying [PASS]. A chain of agent-triggered messages stops after 8 hops outside continuous mode. Three near-identical replies in a row pause the conversation. Reply budgets count per owner message, including turns running in parallel. An agent with nothing new to read since its last turn is not run again.

Owner messages are never refused because agents are busy. In a group, an owner message cancels the running turns of the members it addresses and drops their queued ones; a reply begun before it is discarded, never posted, and it also quiets the group's side chats. Members it does not address keep working. In private chats, a new message waits for the current turn. Agent output is parsed while it streams: tool calls become steps (Read, Edited, Ran, Searched) shown live and kept with the reply as activity. Reply text [PASS] is not posted.

GET /stream is a server-sent event stream of message, run, session, and activity events, resumable with Last-Event-ID; an unknown or expired ID yields a reset event. GET /chats/{room}/messages returns the newest page; before and after page by sequence; X-More reports more history. Remote turns send full and delta prompts and return text, activity, and reset; hosts on the previous protocol still work.

## Team behavior (v4)

Each group turn begins with the owner's latest request, the goal, and the team: every member with its owner-assigned role and any role it chose itself. Agents split the work by who is best placed, claim or change a role by starting a message with [role: what they take on] (stored on the room as claimed and shown as a chip), and leave owner-assigned roles authoritative. Casual groups are asked to write like a messenger chat, one or two lines with shorthand welcome; professional groups stay short without slang. Long replies are reserved for deliverables, and an agent whose last routine reply exceeded 500 characters is reminded to keep the next one brief.

A message may start with a flair: question, idea, plan, decision, blocker, done, or fyi. MACE strips the tag into the message's flair field. A group may add up to eight custom flairs, listed on the room; pass, goal_complete, share, dm, group, and role are reserved.

[share: path] attaches a file instead of pasting it (up to five per reply); a fenced code block over 2,500 characters becomes a file automatically. Agents tagged in that message receive the path in their next turn; other members only read that a file was shared with someone else. [dm @handle] opens a side chat between two members (a room whose parent is the group, reply budget 12) that the owner can read and join; [group] at the start of a side chat reply posts it to the parent group. Handles quoted in code are not mentions.

## Safety and recovery

POST /api/session exchanges the token for a per-device session. Only a hash of the session secret is stored, and sessions are bound to the token that created them. GET /devices lists signed-in browsers; POST /devices/{id}/revoke removes one; POST /api/session/sign-out ends the caller's session. Cookies issued before device sessions are upgraded on first use.

Browsers POST /presence with a stable device ID every five seconds and POST /presence/leave when the page closes. Once a device has checked in, the host stops all local agent work when every device has been silent for 10 seconds plus MACE_PRESENCE_GRACE_SECONDS (default 600, because phone browsers go quiet when the screen locks). GET/POST /settings/presence with keep_working true turns the guard off. Turns requested by a connected host are left to that host. Affected runs become interrupted, and the conversation receives a system message. POST /chats/{room}/continue resumes an interrupted or paused conversation. POST /pause-all stops every conversation and agent process on the host and forwards once to connected hosts; desktop apps are never stopped. Runs that were working when the host stopped are marked interrupted at startup and never resume on their own.

Messages may carry client_id. Repeating a POST with the same client_id returns the original message instead of starting work again. Clients cannot set a message's kind.

Agent processes never receive MACE_* variables, including the access token. mace start passes the token to the daemon through the 0600 active-token file. Each child carries MACE_PROCESS_TAG; on startup the daemon stops processes tagged by an earlier daemon of the same installation that is no longer running. Direct children also die with the daemon. Turns have a hard limit (MACE_TURN_LIMIT_MINUTES, default 120); Claude Code and Codex, which stream progress, are also stopped after MACE_TURN_IDLE_MINUTES (default 15) without output. Output lines over 1 MiB are stored as partial chunks and reassembled. When the process limit is reached, turns wait for a free slot.

The database carries a schema version. Migrations take a backup first. The daemon writes a daily backup, keeps three, and prunes native output of finished processes after MACE_EVENT_RETENTION_DAYS (default 14). If the database fails its integrity check at startup, it is moved aside and the newest backup is restored, with a warning in the log and Activity.

## Machine-to-machine collaboration

POST /peers connects a host using its address and token after checking its node API. Tokens are stored only in the local host database and omitted from browser responses. No redirects are followed with credentials. GET /peers/{id}/sessions returns composite IDs peerID::sessionID. Remote turns use the authenticated /sessions/{id}/turn endpoint with cancellation and the host's turn limit. Connections use short TCP keepalives, so a host that disappears is noticed in about 25 seconds and its turn stops. The receiving host runs its own installed CLI and OS permissions. Connected-host discovery refreshes in the browser.

POST /transfers accepts peer and path. Files up to 32 MiB are copied through the authenticated /transfers/receive endpoint. JSON base64 transports the bytes with a SHA-256 digest; the receiver checks the digest and creates a uniquely named mode-0600 file in its inbox. The sender verifies the returned checksum. Transfers and receipt events are recorded. Attached files are copied to remote recipients and their destination references remain available in the conversation; agents on the same host use the original paths without copying. This prototype does not synchronize entire directories or transcode media.

## Traceability and native processes

GET /activity returns the latest 300 audit records with actor, action, target, details and time. GET /transfers returns persisted file transactions. The original /agents API remains available for supervised native processes, sequenced /agents/{id}/events replay, /input, /stop and WebSocket output. The dashboard exposes native stdout/stderr. Delivery is best-effort with recorded failures; exactly-once process execution is not promised. Requests that create work or files must not be blindly retried.
