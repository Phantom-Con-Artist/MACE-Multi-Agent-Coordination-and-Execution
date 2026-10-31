# MACE client API, version 1

This is the contract between a MACE hub and its apps: the desktop web app and MACE for Android. It is frozen per version. Within version 1, endpoints and fields may be added, never removed, renamed, or changed in meaning. Anything else raises the version.

A test (`server/internal/api/contract_test.go`) checks that every route listed here is served and that this page lists exactly the routes in `server/internal/api/contract.go`. Routes between machines are not part of this contract; they are versioned by the node protocol (`nodeProtocol`, see DESIGN-HUB-WORKER.md).

## Versions

`GET /health` needs no sign-in and answers:

```json
{"application": "mace", "version": "0.1.0", "status": "ok", "api": 1, "node_id": "…", "role": "…", "uptime_seconds": 42}
```

An app compares `api` with the version it was built for. A hub without `api` predates version 1 and is too old. A hub with a higher `api` still serves everything in version 1 unless its release notes say a version was retired.

## Addresses

The hub is reached at its Tailscale HTTPS address (`https://<machine>.<tailnet>.ts.net`, set up by `mace-server setup`) or any address it listens on. All API paths are relative to that origin. Bodies are JSON (`Content-Type: application/json`) unless stated. Errors are plain text in the body, written for people, with these statuses:

| Status | Meaning |
|---|---|
| 400 | The request is malformed. |
| 401 | Not signed in, or the credential was revoked. Sign in again. |
| 403 | Signed in, but this is not allowed (for example a path outside the shared folder). |
| 404 | Not found. |
| 409 | Conflicts with the current state (for example the agent is busy). |
| 502 | Another machine failed to answer. |
| 503 | The hub is starting or its storage is unavailable. |

## Signing in

Browsers hold a `mace_session` cookie. Apps hold a device credential and send it on every request:

```
Authorization: Device dev_0123456789abcdef.0123…
```

Both are per device, appear under Settings, Devices, and stop working when revoked there.

- `POST /api/device` with `{"code": "…", "name": "MACE on Pixel 9"}` returns `{"credential": "dev_….…", "api": 1, "name": "<hub host name>"}`. The code is the one-time value after `#connect-once=` in the sign-in link that the hub shows as a QR (Settings, Phone, or `mace-server qr`); it lasts 10 minutes and works once. A request from one of the owner's own devices on Tailscale may leave out the code. Wrong or used codes get 401 with a message to show.
- `GET /api/session` returns `{"connected": true|false}`; it also signs in a browser on the owner's Tailscale account.
- `POST /api/session` with `{"code": "…"}` signs in a browser (sets the cookie).
- `POST /api/session/sign-out` revokes the calling device.
- `GET /api/v1/signin/qr` returns `{"link": "…", "svg": "<svg…>", "expires_in": 600}`: a sign-in link and its QR for another device.
- `GET /api/v1/devices` lists signed-in devices: `[{"id", "name", "created", "last_seen", "current"}]`.
- `POST /api/v1/devices/{id}/revoke` signs a device out.

## This machine

- `GET /api/v1/node`: this machine's identity, host name, version, role, and active agent count.
- `GET /api/v1/catalog`: the agent CLIs installed here, `{"providers": [{"id", "name", "available", …}]}`.
- `GET /api/v1/metrics`: CPU, memory, and load samples.
- `GET /api/v1/usage`: tokens each agent spent, on every machine: `[{"member", "name", "day", "today": Usage, "total": Usage}]`.
- `GET /api/v1/activity`: the audit log, newest first (`?limit=`).
- `POST /api/v1/pause-all`: pause every running conversation.

`Usage` is `{"input", "cached", "output", "reasoning", "calls", "turns", "weekly_used"}`; counts are tokens as the agent's CLI reported them, `weekly_used` is the account's weekly allowance used, in percent, when the CLI reports one.

## Agents

An agent is a CLI session (agy, Muse, Claude Code, Codex, …) in a working folder. Agents on other machines have IDs of the form `<machine>::<agent>`.

`Session` (an agent) is `{"id", "name", "handle", "provider", "model", "role", "color", "avatar", "working_dir", "status"}`. `status` is one of `ready`, `replying`, `stopped`, `terminated`, `removed`.

- `GET /api/v1/sessions`: agents on this machine.
- `POST /api/v1/sessions` with `{"provider": "agy", "working_dir": "/path"}` starts one; returns the `Session` (201).
- `POST /api/v1/sessions/{id}/control` with `{"action": "stop" | "terminate" | "remove" | "resume"}`.
- `POST /api/v1/sessions/{id}/profile` with any of `{"name", "handle", "role", "color"}`.
- `POST /api/v1/sessions/{id}/command` with `{"name": "model", "arg": "…"}`: a slash command for the CLI.
- `GET /api/v1/sessions/{id}/models`: models the agent's CLI offers.
- `GET /api/v1/avatars/{provider}`: the provider's avatar image.

## Conversations

A conversation is a direct chat (its ID is the agent's ID) or a group (`Room`).

`Room` is `{"id", "name", "members": [agent IDs], "roles": {agent: role}, "claimed": {agent: role}, "flairs", "parent"}`. A room with `parent` is a side chat inside that group.

`Message` is `{"id", "room", "sender", "text", "file", "kind", "flair", "role", "client_id", "at", "seq", "mentions", "mentions_all", "activity", "delivery", "origin"}`. `sender` is `you` for the owner, `mace` for notes from MACE, or an agent ID. `file` is a path under the hub's shared folder; a folder ends in `/`. `activity` lists the agent's steps while replying: `{"id", "kind", "title", "detail", "state", "at"}`. `seq` orders messages within a conversation.

- `GET /api/v1/rooms`, `POST /api/v1/rooms` with `{"name", "members"}`, `DELETE /api/v1/rooms/{room}`.
- `POST /api/v1/rooms/{room}/members` with `{"member"}`; `POST /api/v1/rooms/{room}/members/{member}/role` with `{"role"}`; `POST /api/v1/rooms/{room}/members/{member}/remove`.
- `GET /api/v1/chats/{room}/messages?before=<seq>&after=<seq>&limit=<n>`: newest first page by default, oldest to newest within the page.
- `POST /api/v1/chats/{room}/messages` with `{"text", "file", "client_id"}`. `client_id` makes a retry safe: the same ID is posted once.
- `GET|POST /api/v1/chats/{room}/policy`: `Policy` is `{"mode": "reply" | "goal" | "continuous" | "team", "goal", "limit", "tone", "lead"}`.
- `POST /api/v1/chats/{room}/pause` pauses the conversation and `POST /api/v1/chats/{room}/continue` resumes it. `POST /api/v1/chats/{room}/start` always answers 409: conversations start when the owner writes.
- `GET /api/v1/chats/{room}/receipts`: how far each agent has read, by `seq`.
- `GET /api/v1/runs`: the state of every conversation's work, `Run` is `{"id", "status", "replies", "policy", "error", "active_members", "usage": Usage}`.
- `POST /api/v1/inbox` with `{"seen": {room: seq}}` returns the conversation list: `[{"room", "latest": Message, "unread"}]`.

## Live updates

`GET /api/v1/stream` is a Server-Sent Events stream. Each event has an `id`; reconnect with `Last-Event-ID` to receive what was missed. Events:

| Event | Data |
|---|---|
| `message` | a `Message` posted or updated |
| `room` | a `Room` created or changed |
| `run` | a `Run` changed |
| `session` | a `Session` changed |
| `activity` | `{"room", "member", "steps"}` while an agent replies, then `{"room", "member", "done": true}` |
| `cursor` | an agent read up to a `seq` |
| `records` | another machine's messages arrived for a conversation |
| `metrics` | a new metrics sample |

Clients ignore events they do not know. An open app sends `POST /api/v1/presence` with `{"device": "<a stable ID for this install>"}` every 5 seconds while it is in the foreground; a device without one for `timeout_seconds` counts as gone. The answer is `{"guard", "grace_seconds", "timeout_seconds", "stopped_at"}`: when the guard is on, work stops once every device has been gone for the grace period. `GET|POST /api/v1/settings/presence` reads or sets `{"keep_working": true|false}`, which turns the guard off or on. `POST /api/v1/presence/leave` with the same body says the app closed.

## Other machines

- `GET /api/v1/nodes`: joined machines, `[{"id", "name", "url", "self", "available", "last_seen", "problem", "outdated"}]`.
- `GET /api/v1/peers`: machines whose agents can join conversations.
- `GET|POST /api/v1/peers/{id}/sessions`: list or start agents there (same bodies as `/sessions`).
- `GET /api/v1/peers/{id}/catalog`, `GET /api/v1/peers/{id}/metrics`.
- `GET|POST /api/v1/machines/{id}/{files|apps|host/open|catalog|metrics}`: the same calls as on this machine, answered by that machine through the hub.

## Files and apps

- `GET /api/v1/files?path=<relative>`: a folder listing `[{"name", "path", "directory", "size"}]`, or a file's content (with `Range` support) when `path` is a file.
- `GET /api/v1/apps`: desktop apps installed here; `POST /api/v1/host/open` with `{"app": id}` opens one.

## Notifications

- `GET /api/v1/push/key`, `POST /api/v1/push/subscribe`, `POST /api/v1/push/unsubscribe`: Web Push for browsers. App push is planned as an addition to version 1.

## Planned additions to version 1

These will be added without breaking anything above: uploading a file from a device into a conversation (for the Android share target), registering an app for push notifications, and a per-conversation summary for notification replies.
