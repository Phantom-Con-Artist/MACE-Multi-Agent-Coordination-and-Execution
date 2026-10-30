# Mace collaboration protocol v2

The host is a single-owner trust boundary. Protected endpoints accept the printed Bearer token. POST /api/session exchanges a token for an HttpOnly, SameSite=Strict session cookie. Startup sign-in links put the token in the URL fragment; the browser exchanges it and removes the fragment. Tokens are generated and persisted with mode 0600 unless explicitly configured.

## Agents and host actions

GET /api/v1/catalog discovers supported native CLIs on PATH. POST /sessions accepts provider and optional working_dir; the default is /. GET /sessions returns logical agent instances. Each conversational turn runs a supervised native headless CLI process with that instance's working directory and recent shared transcript. Codex JSONL and Claude JSON are normalized into replies; Antigravity and Muse use their text output modes. Provider credentials and execution permissions remain native to the installed CLI.

POST /sessions/{id}/control accepts action stop, terminate, resume, or remove. Stop cancels relevant runs and processes; terminate marks the instance unavailable; resume makes it ready; remove hides it and removes group memberships while retaining history. Remote composite IDs are supported. POST /rooms/{room}/members adds a member; POST /rooms/{room}/members/{member}/remove pauses the affected run and removes membership without terminating the instance.

GET /apps discovers desktop launchers. POST /host/open accepts an app ID or filesystem path and launches gio/xdg-open under supervision. It opens on the host desktop, not inside a streamed desktop. Launch results and errors are available through native process history and logs. GET /files?path=/ browses permitted files using the daemon user's OS permissions. Files support range requests and native browser media; text previews request the first 64 KiB. MACE_FILE_ROOT can restrict browsing and transfers.

## Conversations

POST /rooms creates a group with name and member IDs. GET/POST /chats/{room}/messages reads or sends messages. A direct conversation uses the logical session ID as its room. sender defaults to you; trusted adapters can specify a member ID. Agent IDs are routing labels, not separately authenticated identities. Text can carry structured JSON data, discussion, or task handoffs. Local file attachments are passed as host paths.

GET/POST /chats/{room}/policy configures mode, limit, and goal. reply mode counts total agent responses across members; its default gives each member one turn. goal mode adds the stated goal to context and stops on an agent's [GOAL_COMPLETE] report or the response budget. The completion marker is a self-report, not independent proof. continuous mode takes turns until cancellation or failure. POST /chats/{room}/pause cancels the active run and its native process group. A session can only reply in one conversation at once. New messages during a running conversation are saved with a delivery error; pause first to send a new task.

GET /runs returns persisted progress, reply counts, completion, failure, or pause status. Runs do not resume automatically after a restart. The orchestrator supplies recent messages (up to 40 and 100,000 bytes) to each turn and records normalized replies in the conversation. Native output remains separately inspectable in Activity. Current history presentation polls persisted state.

## Machine-to-machine collaboration

POST /peers connects a host using its address and token after checking its node API. Tokens are stored only in the local host database and omitted from browser responses. No redirects are followed with credentials. GET /peers/{id}/sessions returns composite IDs peerID::sessionID. Remote turns use the authenticated /sessions/{id}/turn endpoint with cancellation and a ten-minute turn timeout. The receiving host runs its own installed CLI and OS permissions. Connected-host discovery refreshes in the browser.

POST /transfers accepts peer and path. Files up to 32 MiB are copied through the authenticated /transfers/receive endpoint. JSON base64 transports the bytes with a SHA-256 digest; the receiver checks the digest and creates a uniquely named mode-0600 file in its inbox. The sender verifies the returned checksum. Transfers and receipt events are recorded. Attached files are copied to remote recipients and their destination references remain available in the conversation; agents on the same host use the original paths without copying. This prototype does not synchronize entire directories or transcode media.

## Traceability and native processes

GET /activity returns the latest 300 audit records with actor, action, target, details and time. GET /transfers returns persisted file transactions. The original /agents API remains available for supervised native processes, sequenced /agents/{id}/events replay, /input, /stop and WebSocket output. The dashboard exposes native stdout/stderr. Delivery is best-effort with recorded failures; exactly-once process execution is not promised. Requests that create work or files must not be blindly retried.
