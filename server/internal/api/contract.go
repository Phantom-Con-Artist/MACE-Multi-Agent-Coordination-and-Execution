package api

// The client API is what the MACE apps (desktop web, Android) use. It is
// frozen per version: within apiVersion 1, endpoints and fields may be
// added, never removed or changed in meaning. Anything else raises
// apiVersion, and clients compare it with the "api" field of /health.
// docs/API.md describes these routes; a test keeps the router, this list,
// and that document in step. Machine-to-machine routes are versioned
// separately by nodeProtocol.
const apiVersion = 1

var clientRoutes = []string{
	// Health and sign-in
	"GET /health",
	"GET /api/session",
	"POST /api/session",
	"POST /api/session/sign-out",
	"POST /api/device",
	"GET /api/v1/signin/qr",
	"GET /api/v1/devices",
	"POST /api/v1/devices/{id}/revoke",
	// This machine
	"GET /api/v1/node",
	"GET /api/v1/catalog",
	"GET /api/v1/metrics",
	"GET /api/v1/usage",
	"GET /api/v1/activity",
	"POST /api/v1/pause-all",
	// Agents
	"GET /api/v1/sessions",
	"POST /api/v1/sessions",
	"POST /api/v1/sessions/{id}/control",
	"POST /api/v1/sessions/{id}/profile",
	"GET /api/v1/sessions/{id}/photo",
	"POST /api/v1/sessions/{id}/photo",
	"DELETE /api/v1/sessions/{id}/photo",
	"POST /api/v1/chats/{room}/upload",
	"POST /api/v1/chats/{room}/clear",
	"GET /api/v1/rooms/{room}/photo",
	"POST /api/v1/rooms/{room}/photo",
	"DELETE /api/v1/rooms/{room}/photo",
	"POST /api/v1/sessions/{id}/command",
	"GET /api/v1/sessions/{id}/models",
	"GET /api/v1/avatars/{provider}",
	// Conversations
	"GET /api/v1/rooms",
	"POST /api/v1/rooms",
	"DELETE /api/v1/rooms/{room}",
	"POST /api/v1/rooms/{room}/members",
	"POST /api/v1/rooms/{room}/members/{member}/role",
	"POST /api/v1/rooms/{room}/members/{member}/remove",
	"GET /api/v1/chats/{room}/messages",
	"POST /api/v1/chats/{room}/messages",
	"GET /api/v1/chats/{room}/policy",
	"POST /api/v1/chats/{room}/policy",
	"POST /api/v1/chats/{room}/pause",
	"POST /api/v1/chats/{room}/start",
	"POST /api/v1/chats/{room}/continue",
	"GET /api/v1/chats/{room}/receipts",
	"GET /api/v1/runs",
	// Live updates
	"GET /api/v1/stream",
	"POST /api/v1/presence",
	"POST /api/v1/presence/leave",
	"GET /api/v1/settings/presence",
	"POST /api/v1/settings/presence",
	// Other machines
	"GET /api/v1/nodes",
	"GET /api/v1/peers",
	"GET /api/v1/peers/{id}/sessions",
	"POST /api/v1/peers/{id}/sessions",
	"GET /api/v1/peers/{id}/catalog",
	"GET /api/v1/peers/{id}/metrics",
	"GET /api/v1/machines/{id}/*",
	"POST /api/v1/machines/{id}/*",
	// Files and apps
	"GET /api/v1/files",
	"POST /api/v1/inbox",
	"GET /api/v1/apps",
	"POST /api/v1/host/open",
	// Notifications
	"GET /api/v1/push/key",
	"POST /api/v1/push/subscribe",
	"POST /api/v1/push/unsubscribe",
}
