# MACE
Multi Agent Coordination and Execution · prototype 0.1.0

Mace is a self-hosted browser messenger for your machine and its CLI agents. Open the same workspace from a phone, laptop, or PC. Agents run on the host; desktop apps open on the host desktop.

## Use it

Once installed, run:

~~~sh
mace start
~~~

The terminal prints the MACE banner, version, local and LAN sign-in links, access token, and log location. Open a sign-in link; it connects the browser automatically. No token-generation commands or agent launch forms are needed. The public landing page, illustrated setup, Manual, Docs, Troubleshoot, and About remain explorable before sign-in. The interface uses shadcn components, Lucide icons, a self-hosted font, and original vector artwork.

In Chats, tap **+** and choose an installed agent. New sessions default to filesystem root **/**. Tap **Change** to browse to another folder, or choose **Start agent here** while browsing Host files. Multiple instances are named automatically.

Mace discovers Codex, Claude Code, Antigravity (agy), and Muse on the host PATH. Each uses its host installation, login, and permissions. An agent invocation runs for each conversation turn, with the shared transcript supplied by Mace. These are native headless CLI adapters, not terminal emulation.

## Work together

Create a group with local or connected-host agents. Choose Reply limit, Goal oriented, or Continuous mode. The reply limit counts total agent replies, not rounds. Goal mode stops when an agent reports completion with evidence, or its reply budget is reached. Continuous mode runs until paused or a reply fails. A completion report is an agent claim for the owner to inspect.

Conversation controls offer stop, terminate, resume, and remove. Group controls also add or remove members and stop or terminate individual agents. Removal preserves chat history.

## When the connection drops

Mace stops agent work when you are gone. Every signed-in browser checks in every five seconds. If no device has checked in for 10 seconds plus a 15 second grace period, the host stops every agent process it started and marks those conversations Interrupted. When you return, a note says when work stopped, and Continue resumes each conversation. Phone browsers pause in the background, so locking your phone for longer than the grace period also stops work. Set MACE_PRESENCE_GRACE_SECONDS to change the grace period, 0 to stop as soon as possible, or off to keep agents working while you are away. Work started only through the API, with no browser signed in, is not affected.

Pause everything, in the workspace banner or with `mace pause-all`, stops all agent work on this host and on connected hosts. Desktop apps are never stopped. A host restart also marks running work Interrupted, so nothing resumes on its own.

Each agent has a name, an @mention name, a role, and an avatar color. Change them from the agent's private chat menu under Name, mention and role. The role goes with the agent into every group unless a group assigns its own. Claude Code and Muse avatars use their own marks. For other agents, place an official logo you may use at avatars/<provider>.svg (or .png, .webp) in the Mace state folder, for example ~/.config/mace/avatars/agy.svg.

Each browser keeps its own sign-in. Settings lists signed-in devices and lets you remove one. Changing the access token signs out every device.

Host provides file browsing, text/media inspection, desktop app discovery and launch, and agent controls. Activity shows conversation state, audit events, native stdout/stderr, and file transactions.

## Your machines: a hub and workers

Every machine you use runs MACE and keeps its own data. One of them is the hub your phone opens; the others are workers whose agents join your chats. All of them must be logged in to the same Tailscale account.

On the hub, once:

~~~sh
sudo tailscale set --operator=$USER   # lets MACE publish over HTTPS
mace-server setup                     # runs the hub as a service at https://<machine>.<tailnet>.ts.net
~~~

On each worker, once:

~~~sh
mace setup                            # runs MACE as a service that starts at boot
~~~

That is all. Every MACE looks for MACE on your other Tailscale devices and joins them by itself, so it does not matter which machine starts first: a worker that is switched on later joins as soon as it is online. Tailscale vouches that a device is yours, so no codes are needed. Devices of other people on a shared tailnet, devices shared in from other tailnets, and tagged devices are never trusted. A machine you remove stays removed until you join it again with a code (`mace join --invite` on one, `mace join <address> <code>` on the other), which also works for machines that are not on Tailscale. Set MACE_TAILSCALE_TRUST=off to turn trust through Tailscale off.

On your phone, open the hub's address with Tailscale on: it signs in by itself. Then use Add to Home Screen. For a device on another account, `mace-server qr` or Settings, Add a phone or tablet, shows a one-time QR code. HTTPS needs MagicDNS and HTTPS certificates turned on in the Tailscale admin console (DNS page). `mace-server start` does the same as setup without installing a service.

Once joined, a worker's agents appear in the hub's agent list, can be added to groups, and answer through the hub; the hub signs every request with its machine key. Chatting with a worker's agent from the hub and directly on the worker shows one history on both, marked with the machine each message came from. Files move between machines as SHA-256 chunks, so a dropped connection resumes and there is no size limit. When a machine is offline, chats say so and its agents answer once it is back. Settings, Your machines, opens another machine already signed in. `mace machines` lists them and who is available.

Notifications: in Settings, turn on Notifications on this device. You hear about private replies, questions, blockers, plans, decisions, and finished work; routine group chat stays quiet. They need the HTTPS address from mace-server start.

Hosts that are not joined can still be connected the older way, by address and access token, under Settings, Your machines, Hosts connected by token.

## Context and quota

Agents spend quota on your work, not on re-reading MACE. A resumed agent gets the team, roles, and rules again only when they change. It can fetch more on demand with `mace ctx team | plan | recent | thread @handle | search <words>`, which repeats nothing it already has. If graphify is installed (`uv tool install graphifyy`), agents working in a git repository also get a code graph to query before grepping; MACE builds it without any model calls.

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
mace pause-all          # stop all agent work here and on connected hosts
mace join --invite      # print a one-time code for adding another machine
mace machines           # joined machines and who is available
mace-server setup       # run this machine as the hub service, over tailnet HTTPS
mace-server start       # the same, without installing a service
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
