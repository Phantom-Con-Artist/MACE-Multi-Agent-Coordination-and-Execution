# MACE
Multi Agent Coordination and Execution · prototype 0.1.0

Mace is a self-hosted browser messenger for your machine and its CLI agents. Open the same workspace from a phone, laptop, or PC. Agents run on the host; desktop apps open on the host desktop.

## Use it

Once installed, run:

~~~sh
mace start
~~~

The terminal prints the MACE banner, version, local and LAN sign-in links, access token, and log location. Open a sign-in link; it connects the browser automatically. No token-generation commands or agent launch forms are needed. The public landing page, illustrated setup, Manual, Docs, Troubleshoot, and About remain explorable before sign-in. The interface uses shadcn components, Lucide icons, a self-hosted font, and original Codex Imagen artwork.

In Chats, tap **+** and choose an installed agent. New sessions default to filesystem root **/**. Tap **Change** to browse to another folder, or choose **Start agent here** while browsing Host files. Multiple instances are named automatically.

Mace discovers Codex, Claude Code, Antigravity (agy), and Muse on the host PATH. Each uses its host installation, login, and permissions. An agent invocation runs for each conversation turn, with the shared transcript supplied by Mace. These are native headless CLI adapters, not terminal emulation.

## Work together

Create a group with local or connected-host agents. Choose Reply limit, Goal oriented, or Continuous mode. The reply limit counts total agent replies, not rounds. Goal mode stops when an agent reports completion with evidence, or its reply budget is reached. Continuous mode runs until paused or a reply fails. A completion report is an agent claim for the owner to inspect.

Conversation controls offer stop, terminate, resume, and remove. Group controls also add or remove members and stop or terminate individual agents. Removal preserves chat history. Runs are paused after a host restart, so work never silently resumes.

Host provides file browsing, text/media inspection, desktop app discovery and launch, and agent controls. Activity shows conversation state, audit events, native stdout/stderr, and file transactions.

To collaborate across machines, run Mace on each host, connect the other address and token in Connected hosts, and add its agents to your groups. Local agents receive file paths directly. Remote attachments are copied through Mace with SHA-256 verification, up to 32 MiB each. Both hosts must be reachable through LAN, Tailnet, or your configured private network.

~~~sh
mace status
mace info               # current addresses, sign-in links, and token
mace doctor             # port, paths, and installed-agent checks
mace agents             # conversational instances
mace ps                 # native processes
mace logs --lines 50
mace logs --follow
mace restart
mace version
mace stop
mace kill
maced                  # foreground daemon
mace maced             # same foreground entry point
~~~

Startup waits for the actual HTTP health check before printing success. Occupied ports receive an actionable error; `mace start --port 0` selects a free port. Duplicate starts report the existing server. Stale PID records never authorize signalling an unrelated process. Stop and kill are safe to repeat. Lifecycle locking recovers automatically when a CLI process exits. Restart keeps the saved address, database, and access token. Forced shutdown also stops native descendants.

## Build and install

Requirements: Go matching go.mod, Node.js/npm, and a Linux host. Browser clients can use other operating systems.

~~~sh
make build
make install
~~~

The default install prefix is ~/.local. Add ~/.local/bin to your PATH if your shell does not include it. Installation includes the web assets. Repository builds can use ./bin/mace start directly; their web assets are discovered next to the binary. Installed Mace can start from any working directory.

Persistent state defaults to the user's configuration directory under mace/; on Linux this is usually ~/.config/mace. A randomly generated token is persisted with mode 0600. MACE_AUTH_SECRET overrides it. MACE_HOME selects another state location. Existing databases can be selected with MACE_DB_PATH. Environment examples are in .env.example; Mace does not source that file automatically.

The default listener is 0.0.0.0:8080. MACE_HTTP_HOST and MACE_HTTP_PORT override it. The file browser defaults to the host filesystem root and operates with the daemon user's permissions; MACE_FILE_ROOT can restrict the browsing root. Desktop launch requires an active graphical session with gio/xdg-open.

MACE_TS_AUTHKEY, MACE_TS_HOSTNAME, and MACE_TS_STATE_DIR configure embedded Tailscale. Actual enrollment and cross-device Tailnet access require your own credentials.

## Development and verification

~~~sh
go test ./...
go test -race ./internal/api ./internal/agent
cd web && npm run build
~~~

The Vite development server proxies to port 8080. scripts/check-browser.mjs uses Playwright and starts two isolated test daemons on ports 18080/18081, then cleans them up. Set MACE_PLAYWRIGHT to a Playwright module entry point if it is not installed locally, and MACE_BROWSER to a Chromium executable if necessary. Its diagnostic provider is enabled only by MACE_TEST_MODE=1; it never calls an AI service.

See [protocols](docs/MESSENGER-PROTOCOL.md) and [verification](docs/VERIFICATION.md). Live provider accounts, GUI apps on the owner's desktop, and actual remote networking require environment-specific verification.
