// MACE service worker: lets the workspace install to a home screen and shows
// notifications when an agent needs the owner. Pages and API calls always go
// to the network, so nothing stale is ever shown.

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      // Someone is looking at MACE right now; the message is already on screen.
      if (clients.some((c) => c.visibilityState === "visible")) return;
      return self.registration.showNotification(data.title || "MACE", {
        body: data.body || "",
        tag: data.tag || "mace",
        icon: "/brand/mace-icon.svg",
        badge: "/brand/mace-mark.svg",
        data: { room: data.room || "" },
      });
    }),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const room = event.notification.data && event.notification.data.room;
  const url = "/" + (room ? "?room=" + encodeURIComponent(room) : "") + "#messenger";
  event.waitUntil(
    self.clients.matchAll({ type: "window", includeUncontrolled: true }).then((clients) => {
      for (const c of clients) {
        if ("focus" in c) {
          c.postMessage({ type: "open-room", room });
          return c.focus();
        }
      }
      return self.clients.openWindow(url);
    }),
  );
});
