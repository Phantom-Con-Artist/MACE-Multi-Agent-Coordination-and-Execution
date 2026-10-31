import { useEffect, useMemo, useState } from "react";
import { MaceTile } from "./brand";

// Agents get a tile from the MACE mark: the triangle of their provider's
// initial where it exists in M-A-C-E, the E tile (message pills) for groups.
// Tile and shape colors alternate by identity so a list never looks uniform.
const letterFor = (provider?: string, group?: boolean) =>
  group
    ? "E"
    : ({ muse: "M", agy: "A", codex: "C", claude: "C", diagnostic: "E" } as Record<string, string>)[
        provider || ""
      ] || "A";

const hash = (s: string) => [...s].reduce((h, c) => (h * 31 + c.charCodeAt(0)) >>> 0, 7);

export function Avatar({
  id,
  provider,
  group = false,
  size = 44,
  working = false,
}: {
  id: string;
  provider?: string;
  group?: boolean;
  size?: number;
  working?: boolean;
}) {
  return (
    <span
      className={"tile-avatar" + (working ? " working" : "")}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <MaceTile letter={letterFor(provider, group)} inverse={hash(id) % 2 === 1} />
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

/** One readable line from Markdown, for chat list previews. */
export function plainPreview(text: string) {
  return text
    .replace(/```[\s\S]*?```/g, "code")
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, "$1")
    .replace(/[*_`~#>]+/g, "")
    .replace(/\s+/g, " ")
    .trim();
}
