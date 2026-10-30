# Mace product constraints

- Product name: MACE, Multi Agent Coordination and Execution. Current requested version: 0.1.0.
- Do not use em dashes in product copy, documentation, or user-facing messages.
- The landing page, Manual, Docs, Troubleshoot, and About must remain explorable without signing in. The workspace requires host authentication.
- The primary user flow is mace start, open its printed sign-in link, choose an installed CLI agent, and chat. Do not require launch arguments, JSON, or an executable form for ordinary users.
- Agent sessions default to filesystem root /. Offer a browseable folder picker and Start agent here. This means a working directory, not superuser privileges.
- Preserve direct chat, local and remote groups, goal/reply/continuous modes, pause, stop, terminate, resume, removal, file inspection, desktop app launch, host connections, and activity logs.
- Take inspiration from familiar messengers without copying WhatsApp's branding, wallpaper, or visual treatment.
- Use shadcn components, Lucide utility icons, self-hosted typography, geometric artwork, rounded controls, and solid colors. Keep native scrolling, mobile layouts, keyboard focus, and truthful empty/error states.
- Art prompts and references are recorded in docs/ART-DIRECTION.md. Do not replace generated artwork with generic decorative placeholders.
- Live provider accounts, real GUI launches, and real Tailnet enrollment must not be reported as verified by diagnostic fixtures or two localhost daemons.
