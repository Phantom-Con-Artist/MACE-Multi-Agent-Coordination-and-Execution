# Mace 0.1.0 verification

Verified on the local Linux host on 2026-10-30 and 2026-10-31. Tests use isolated state directories and leave the test daemons stopped.

## Actual live agents

scripts/check-live-agents.mjs ran the installed agy and muse executables using their real host accounts. Antigravity returned MACE_AGY_LIVE_OK and Muse returned MACE_MUSE_LIVE_OK through Mace's conversation API. A real group then received two native responses: Antigravity MACE_GROUP_OK and Muse Code MACE_GROUP_OK. The browser rendered the resulting group history. The prompts prohibited tools and file reads or writes.

The opened screenshot is /tmp/mace-live-agents.png. Live state and logs are retained under /tmp/mace-live-5lmHUc. The live test host was stopped after verification. These are actual provider calls, distinct from diagnostic fixtures.

## Compiler, language server, and concurrency

go test -count=1 ./... passed. go test -race ./internal/api ./internal/agent passed. Native adapter fixture tests cover Codex JSONL, Claude JSON, Antigravity text, and Muse text. Group tests cover bounded replies, goal reports, continuous cancellation, lifecycle controls, and retained history.

The editor screenshot showed stale missing-field diagnostics for APIHandler.fileRoot. The field exists in handlers.go. A fresh gopls v0.24.0 check of handlers.go, collaboration_test.go, and workspace.go returned no diagnostics after minor string-builder hints were corrected. No missing-field compiler error was reproduced.

## Real CLI edge cases

scripts/check-cli.mjs passed using real socket listeners and daemon processes. It checks help and version output, invalid ports and missing flag values, occupied non-Mace ports, ports already serving Mace, automatic free-port selection, duplicate starts, stale PID records belonging to an unrelated Node process, stale lock files, and HTTP health failure while a daemon is suspended.

It also checks info/token output, logs and invalid log limits, agent and native process listings, restart with persistent token and database, forced cleanup of a native shell and sleep process tree, repeated stop/kill, a missing daemon executable, and failed-start cleanup for an invalid daemon flag. The test starts from /tmp, outside the repository.

Startup waits for its own healthy node, rather than a fixed delay. PID ownership is checked against the actual daemon executable, and zombie processes are treated as stopped. Locks use kernel advisory locking so stale files do not block future starts.

## Two-host and browser workflows

The integration suite uses two separate HTTP hosts and SQLite stores to verify authenticated host connection, remote session discovery, browser-encoded composite agent IDs, cross-host replies and lifecycle controls, and file copy with SHA-256 verification. Received files are compared to their original bytes. The test also verifies that peer tokens are omitted from API responses.

The browser script checks the public landing page without a session, three-step illustrated setup tabs, public Manual/Docs/Troubleshoot/About, troubleshooting search and disclosures, mobile navigation and dark appearance. It then checks a discovered agent, folder selection, chat, stop/terminate/resume, goal and continuous groups, pause, member removal, another host's chat, activity, and responsive routes at 320, 768 and 1280px.

Opened screenshots include /tmp/mace-home-desktop.png, /tmp/mace-home-phone.png, /tmp/mace-setup-guide.png, /tmp/mace-home-dark-phone.png, /tmp/mace-v2-phone.png, and /tmp/mace-v2-activity.png. The public-site skill informed illustrated instructions and public navigation. The UI skill informed single-pane phone conversations, saved drafts, and visible cancellation/error states. Codex Imagen generated the original host and setup artwork; prompts are in docs.

npm run build passed with TypeScript, shadcn components, and the production Vite build. git diff --check passed. Product copy was checked for em dashes.

## Environment-specific checks still open

Real Tailnet enrollment and communication between physically separate devices require owner networking credentials and have not been exercised. The two-host test uses separate daemons on localhost. Real GUI app launch on the owner's desktop has not been exercised. Media codecs depend on the browser. Codex and Claude account-backed requests were not run; their adapters were verified with fixtures. Full screen-reader, text-zoom, and rendered-contrast audits are not claimed.
