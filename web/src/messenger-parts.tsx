import { useEffect, useMemo, useState } from "react";
import { MaceTile } from "./brand";

// Avatars: the agent's own mark where Mace may ship it (Claude Code, Muse), a
// logo the owner provided, or otherwise the MACE tile of its initial. Groups
// get the E tile, whose pills read as people talking. Each instance has a
// color profile, so agy and agy-2 are easy to tell apart.
export const colorProfiles = ["Red", "Ink", "Paper", "Maroon", "Blush", "Charcoal"];
const shipped: Record<string, string> = { claude: "/agents/claude.svg", muse: "/agents/muse.svg" };
const letterFor = (provider?: string) =>
  ({ muse: "M", agy: "A", codex: "C", claude: "C", diagnostic: "E" } as Record<string, string>)[provider || ""] || "A";

const hash = (s: string) => [...s].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7);

// Instance order: red first, then the most distinct neighbors, ink last
// because it disappears on dark themes.
const instanceOrder = [0, 2, 3, 4, 5, 1];

/** The default profile follows the instance number in the handle: agy, agy-2, agy-3. */
export function profileFor(color: number | undefined | null, handle: string | undefined, _id: string) {
  if (color !== undefined && color !== null) return color;
  const n = /-(\d+)$/.exec(handle || "");
  return instanceOrder[((n ? Number(n[1]) : 1) - 1) % instanceOrder.length];
}

export function Avatar({
  id,
  provider,
  group = false,
  size = 44,
  working = false,
  color,
  handle,
  ownerLogo = false,
}: {
  id: string;
  provider?: string;
  group?: boolean;
  size?: number;
  working?: boolean;
  color?: number;
  handle?: string;
  ownerLogo?: boolean;
}) {
  const profile = group ? hash(id) % 2 : profileFor(color, handle, id);
  const mark = !group && provider ? shipped[provider] : undefined;
  return (
    <span
      className={"agent-avatar profile-" + profile + (working ? " working" : "")}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      {group ? (
        <MaceTile letter="E" inverse={profile === 1} />
      ) : ownerLogo && provider ? (
        <img src={"/api/v1/avatars/" + provider} alt="" />
      ) : mark ? (
        <i className="agent-mark" style={{ maskImage: `url(${mark})`, WebkitMaskImage: `url(${mark})` }} />
      ) : (
        <MaceTile letter={letterFor(provider)} glyph />
      )}
    </span>
  );
}

/** A host app: its initials on an avatar color, with the logo's triangle cut. */
export function AppTile({ name, size = 56 }: { name: string; size?: number }) {
  const words = name.replace(/[^\p{L}\p{N} ]/gu, " ").split(/\s+/).filter(Boolean);
  const initials = (words.length > 1 ? words[0][0] + words[1][0] : (words[0] || "?").slice(0, 2)).toUpperCase();
  return (
    <span className={"app-tile profile-" + (hash(name) % 6)} style={{ width: size, height: size }} aria-hidden="true">
      <svg viewBox="0 0 56 56">
        <path d="M56 0 L56 30 L30 0 Z" className="app-tile-cut" />
      </svg>
      <b>{initials}</b>
    </span>
  );
}

const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

export function dayLabel(at: string) {
  const d = new Date(at),
    today = day(new Date()),
    diff = (today - day(d)) / 86400000;
  if (diff === 0) return "Today";
  if (diff === 1) return "Yesterday";
  if (diff < 7) return d.toLocaleDateString([], { weekday: "long" });
  return d.toLocaleDateString([], { day: "numeric", month: "long", year: d.getFullYear() === new Date().getFullYear() ? undefined : "numeric" });
}

export function listTime(at: string) {
  const d = new Date(at);
  return day(d) === day(new Date())
    ? d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })
    : dayLabel(at);
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Turns @handle into a Markdown link the renderer shows as a mention. */
export function markMentions(text: string, handles: string[]) {
  if (!handles.length) return text;
  const pattern = new RegExp(
    `(^|[^A-Za-z0-9_\\[])@(${[...handles, "all"].map(escape).join("|")})(?![A-Za-z0-9_-])`,
    "gi",
  );
  return text.replace(pattern, (_, pre, h) => `${pre}[@${h}](mace-mention:${h})`);
}

/** Plain text with @mentions highlighted, for the owner's own messages. */
export function PlainWithMentions({ text, handles }: { text: string; handles: string[] }) {
  const parts = useMemo(() => {
    if (!handles.length) return [text];
    const pattern = new RegExp(`(@(?:${[...handles, "all"].map(escape).join("|")}))(?![A-Za-z0-9_-])`, "gi");
    return text.split(pattern);
  }, [text, handles]);
  return (
    <>
      {parts.map((p, i) =>
        i % 2 ? (
          <span key={i} className="mention">
            {p}
          </span>
        ) : (
          p
        ),
      )}
    </>
  );
}

export type Suggestion = { id: string; handle: string; name: string; detail: string; provider?: string };

/**
 * The @ menu. It reads the word at the caret and offers matching coworkers,
 * keyboard first: arrows move, Enter or Tab inserts, Escape closes.
 */
export function useMentionMenu(
  value: string,
  caret: number,
  people: Suggestion[],
) {
  const [dismissed, setDismissed] = useState("");
  const [active, setActive] = useState(0);
  const before = value.slice(0, caret);
  const match = /(^|\s)@([A-Za-z0-9._@-]*)$/.exec(before);
  const query = match ? match[2].toLowerCase() : null;
  const key = match ? before.length - match[2].length + ":" + match[2] : "";
  const options =
    query === null || key === dismissed
      ? []
      : people.filter(
          (p) =>
            p.handle.toLowerCase().startsWith(query) ||
            p.name.toLowerCase().includes(query),
        ).slice(0, 6);
  useEffect(() => setActive(0), [query]);
  const insert = (s: Suggestion) => {
    const start = before.length - (match ? match[2].length : 0);
    const next = value.slice(0, start) + s.handle + " " + value.slice(caret);
    return { next, caret: start + s.handle.length + 1 };
  };
  return {
    open: options.length > 0,
    options,
    active,
    move: (d: number) => setActive((a) => (a + d + options.length) % options.length),
    dismiss: () => setDismissed(key),
    insert,
  };
}

/** Commands offered at the start of a message in a private chat. */
export const chatCommands: { name: string; args?: string; detail: string }[] = [
  { name: "model", args: "<name>", detail: "Set the model everywhere this agent works" },
  { name: "models", detail: "List models and show the current one" },
  { name: "cd", args: "<folder>", detail: "Change the working folder" },
  { name: "new", detail: "Start a fresh native session" },
  { name: "stop", detail: "Stop the current turn and clear waiting work" },
  { name: "status", detail: "Model, folder, waiting work, and groups" },
  { name: "rename", args: "<handle>", detail: "Change the mention handle" },
  { name: "files", detail: "Open host files at the agent's folder" },
  { name: "help", detail: "List commands" },
];

/** A known command at the start of a draft, or null for an ordinary message. */
export function parseCommand(text: string) {
  const m = /^\/([a-z]+)(?:\s+([\s\S]*))?$/.exec(text.trim());
  if (!m || !chatCommands.some((c) => c.name === m[1])) return null;
  return { name: m[1], arg: (m[2] || "").trim() };
}

export type CommandOption = {
  id: string;
  label: string;
  detail: string;
  kind: "command" | "model" | "folder";
  /** The whole draft after picking. */
  text: string;
  /** Picking runs the command at once instead of waiting for more input. */
  run?: boolean;
};

const joinPath = (dir: string, name: string) => (dir.endsWith("/") ? dir : dir + "/") + name;

/**
 * The / menu. It lists commands while the first word is typed, then models
 * after /model and folders after /cd. Same keys as the @ menu.
 */
export function useCommandMenu(
  value: string,
  caret: number,
  enabled: boolean,
  agent: { id: string; folder: string } | null,
  load: <T>(path: string) => Promise<T>,
) {
  const [dismissed, setDismissed] = useState("");
  const [active, setActive] = useState(0);
  const [models, setModels] = useState<{ agent: string; list: { id: string; label?: string }[]; current: string } | null>(null);
  const [folders, setFolders] = useState<{ dir: string; names: string[] }>({ dir: "", names: [] });
  const atEnd = caret >= value.length;
  const word = enabled && atEnd ? /^\/([a-z]*)$/.exec(value) : null;
  const model = enabled && atEnd ? /^\/model\s+(\S*)$/.exec(value) : null;
  const cd = enabled && atEnd ? /^\/cd\s+(.*)$/.exec(value) : null;

  // The folder being browsed, absolute, and the partial name after it.
  let dir = "",
    partial = "",
    typedDir = "";
  if (cd && agent) {
    const arg = cd[1];
    const cut = arg.lastIndexOf("/");
    typedDir = cut >= 0 ? arg.slice(0, cut + 1) : "";
    partial = cut >= 0 ? arg.slice(cut + 1) : arg;
    dir = typedDir.startsWith("/") ? typedDir || "/" : joinPath(agent.folder, typedDir);
  }
  useEffect(() => {
    if (!model || !agent || models?.agent === agent.id) return;
    load<{ models: { id: string; label?: string }[]; current: string }>("/v1/sessions/" + encodeURIComponent(agent.id) + "/models")
      .then((r) => setModels({ agent: agent.id, list: r.models, current: r.current }))
      .catch(() => setModels({ agent: agent.id, list: [], current: "" }));
  }, [!!model, agent?.id]);
  useEffect(() => {
    if (!cd || folders.dir === dir) return;
    let alive = true;
    load<{ name: string; directory: boolean }[]>("/v1/files?path=" + encodeURIComponent(dir))
      .then((r) => alive && setFolders({ dir, names: r.filter((e) => e.directory && !e.name.startsWith(".")).map((e) => e.name).sort() }))
      .catch(() => alive && setFolders({ dir, names: [] }));
    return () => {
      alive = false;
    };
  }, [cd ? dir : ""]);

  let options: CommandOption[] = [];
  if (word) {
    options = chatCommands
      .filter((c) => c.name.startsWith(word[1]))
      .map((c) => ({
        id: c.name,
        label: "/" + c.name + (c.args ? " " + c.args : ""),
        detail: c.detail,
        kind: "command",
        text: "/" + c.name + (c.args ? " " : ""),
        run: !c.args,
      }));
  } else if (model && models && models.agent === agent?.id) {
    const q = model[1].toLowerCase();
    const current = models.current || "default";
    options = [{ id: "default", label: "CLI default" }, ...models.list]
      .filter((m) => m.id.toLowerCase().includes(q) || (m.label || "").toLowerCase().includes(q))
      .map((m) => ({
        id: m.id,
        label: m.id,
        detail: [m.label !== m.id ? m.label : "", m.id === current ? "current" : ""].filter(Boolean).join(" · "),
        kind: "model",
        text: "/model " + m.id,
        run: true,
      }));
  } else if (cd && folders.dir === dir) {
    const q = partial.toLowerCase();
    const matches = folders.names.filter((n) => n.toLowerCase().startsWith(q)).slice(0, 7);
    options = matches.map((n) => ({
      id: n,
      label: n + "/",
      detail: joinPath(dir, n),
      kind: "folder",
      text: "/cd " + typedDir + n + "/",
    }));
    if (!partial && cd[1] && matches.length)
      options.unshift({ id: ".", label: "Use this folder", detail: dir, kind: "folder", text: value, run: true });
  }
  const key = word ? "w:" + word[1] : model ? "m:" + model[1] : cd ? "c:" + cd[1] : "";
  if (key === dismissed) options = [];
  useEffect(() => setActive(0), [key]);
  return {
    open: options.length > 0,
    options,
    active: Math.min(active, Math.max(0, options.length - 1)),
    move: (d: number) => setActive((a) => (a + d + options.length) % options.length),
    dismiss: () => setDismissed(key),
  };
}

/** One readable line from Markdown, for chat list previews. */
export function plainPreview(text: string) {
  return text
    .replace(/```[\s\S]*?```/g, "code")
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[*_`~#>]+/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
