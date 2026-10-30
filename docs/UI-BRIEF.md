# Mace messenger

The owner controls a single host from a phone, PC, or laptop. Their explicit direction is “exactly similar like whatsapp”, mobile first, light and dark, with a homepage, manual, messenger, configuration and settings. The existing stack is Go, SQLite, React and Vite; the React source was absent at discovery. Rain's PRODUCT.md establishes the useful precedent: one conversation pane on phones, explicit working directories, groups independent from process lifecycles, native media playback.

The UI uses the requested familiar chat hierarchy: navigation rail, conversation list, selected thread, bottom composer. Mobile shows list or thread and preserves per-thread drafts. Green, rounded filters, system typography and message bubbles follow the user's explicit WhatsApp direction. Empty states contain no fictional agents or messages. A persistent connection error points to Settings. Message failures preserve the saved message and show a delivery issue.

The strongest objection is that this could be a themed shell. The implementation answers it with persisted rooms and messages, executable launch, process stdin delivery, output replay, process stop, host-root file browsing and native authenticated media. CLI transports currently use newline-delimited pipes; PTY-only interfaces need a future adapter. Group forwarding is explicit to avoid unbounded autonomous loops.

State: implementation complete; verification recorded in the delivery notes. Browser layout checks, process tests and manual lifecycle checks are required before claiming a full remote deployment. Tailscale enrollment requires owner credentials and remains external verification.
