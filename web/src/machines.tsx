import { useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowLeft, ChevronLeft, ChevronRight, FileText, Folder, Monitor, Plus, Search, Terminal } from "lucide-react";
import { MachineCard, usePeerMetrics, type Sample } from "./charts";
import { AppTile } from "./messenger-parts";

// Your machines: this computer and every joined worker, one card each.
// Cards swipe on touch screens and step with the arrows on desktop; the
// selected machine's agents, folders, and apps sit below, so browsing a
// worker reads exactly like browsing this computer.

export type Machine = { id: string; name: string; self: boolean; available: boolean };
type Entry = { name: string; path: string; directory: boolean; size: number };
type App = { id: string; name: string; category?: string };
type Provider = { id: string; name: string; available: boolean };
type Tab = "agents" | "files" | "apps";

/** Where a machine's API answers: here, or forwarded by the hub. */
export const machineBase = (m: Machine) =>
  m.self ? "/api/v1" : "/api/v1/machines/" + encodeURIComponent(m.id);

const full = (p: string) => (p === "." ? "/" : "/" + p.replace(/^\/+/, ""));

async function get<T>(url: string): Promise<T> {
  const r = await fetch(url);
  if (!r.ok) throw Error((await r.text()).trim() || r.statusText);
  return r.json();
}

function RemoteCard({ machine, active }: { machine: Machine; active: boolean }) {
  const m = usePeerMetrics(machine.id, active && machine.available);
  return (
    <MachineCard
      name={machine.name}
      history={m.history}
      error={!machine.available ? "Offline. Its agents answer when it is back." : m.error}
    />
  );
}

export function Machines(props: {
  machines: Machine[];
  localHistory: Sample[];
  agentIDs: string[];
  agentRow: (id: string) => ReactNode;
  run: (fn: () => Promise<void>) => void;
  busy: boolean;
  onStart: (m: Machine, provider: string, folder: string) => Promise<void>;
  onPreview: (m: Machine, path: string) => void;
  onLaunched: (m: Machine) => void;
}) {
  const { machines } = props;
  const strip = useRef<HTMLDivElement>(null);
  const [index, setIndex] = useState(0);
  const [tab, setTab] = useState<Tab>("agents");
  const [paths, setPaths] = useState<Record<string, string>>({});
  const [entries, setEntries] = useState<Entry[] | null>(null);
  const [apps, setApps] = useState<App[] | null>(null);
  const [providers, setProviders] = useState<Provider[] | null>(null);
  const [search, setSearch] = useState("");
  const [starting, setStarting] = useState(false);
  const [problem, setProblem] = useState("");
  const current = machines[Math.min(index, machines.length - 1)];
  const base = current ? machineBase(current) : "";
  const path = (current && paths[current.id]) || ".";

  // The card nearest the strip's center is the selected machine.
  const onScroll = () => {
    const el = strip.current;
    if (!el) return;
    const next = Math.round(el.scrollLeft / Math.max(1, el.clientWidth));
    if (next !== index) setIndex(Math.max(0, Math.min(machines.length - 1, next)));
  };
  const go = (i: number) => {
    const el = strip.current;
    const target = Math.max(0, Math.min(machines.length - 1, i));
    el?.scrollTo({
      left: target * el.clientWidth,
      behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth",
    });
    setIndex(target);
  };

  // Each machine opens on its agents, with fresh folders, apps, and CLIs.
  useEffect(() => {
    setEntries(null);
    setApps(null);
    setProviders(null);
    setStarting(false);
    setProblem("");
    setSearch("");
  }, [current?.id]);
  useEffect(() => {
    if (!current?.available) return;
    let alive = true;
    const fail = (e: Error) => alive && setProblem(e.message);
    if (tab === "files")
      void get<Entry[]>(base + "/files?path=" + encodeURIComponent(path))
        .then((list) =>
          alive &&
          setEntries(
            list.sort((a, b) => Number(b.directory) - Number(a.directory) || a.name.localeCompare(b.name)),
          ),
        )
        .catch(fail);
    if (tab === "apps" && !apps)
      void get<App[]>(base + "/apps").then((l) => alive && setApps(l)).catch(fail);
    if (tab === "agents" && starting && !providers)
      void get<{ providers: Provider[] }>(base + "/catalog")
        .then((c) => alive && setProviders(c.providers.filter((p) => p.available)))
        .catch(fail);
    return () => {
      alive = false;
    };
  }, [current?.id, current?.available, tab, path, starting]);

  const open = (p: string) => {
    setProblem("");
    setEntries(null);
    setPaths((v) => ({ ...v, [current.id]: p }));
  };
  const launchApp = (id: string) =>
    props.run(async () => {
      const r = await fetch(base + "/host/open", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ app: id }),
      });
      if (!r.ok) throw Error((await r.text()).trim());
      props.onLaunched(current);
    });

  if (!current) return null;
  const mine = props.agentIDs.filter((id) => (current.self ? !id.includes("::") : id.startsWith(current.id + "::")));
  const count = machines.length;

  return (
    <div className="machines-view">
      <div className="machine-nav">
        <button className="icon-button machine-step" aria-label="Previous machine" disabled={index === 0} onClick={() => go(index - 1)}>
          <ChevronLeft size={20} />
        </button>
        <div className="machine-where" aria-live="polite">
          <strong>{current.self ? "This computer" : current.name}</strong>
          <small>
            {index + 1} of {count}
          </small>
        </div>
        <button className="icon-button machine-step" aria-label="Next machine" disabled={index >= count - 1} onClick={() => go(index + 1)}>
          <ChevronRight size={20} />
        </button>
      </div>
      <div
        className="machine-strip"
        ref={strip}
        onScroll={onScroll}
        tabIndex={0}
        aria-label="Machines. Swipe or use the arrow keys to switch."
        onKeyDown={(e) => {
          if (e.key === "ArrowRight") go(index + 1);
          if (e.key === "ArrowLeft") go(index - 1);
        }}
      >
        {machines.map((m, i) => (
          <div className={"machine-slide" + (i === index ? " current" : "")} key={m.id} aria-hidden={i !== index}>
            {m.self ? (
              <MachineCard name={m.name} history={props.localHistory} />
            ) : (
              <RemoteCard machine={m} active={i === index} />
            )}
          </div>
        ))}
      </div>
      {count > 1 && (
        <div className="machine-dots" aria-hidden="true">
          {machines.map((m, i) => (
            <i key={m.id} className={i === index ? "on" : ""} />
          ))}
        </div>
      )}

      <div className="machine-tabs" role="tablist" aria-label={"What is on " + current.name}>
        {(
          [
            ["agents", "Agents", Terminal],
            ["files", "Files", Folder],
            ["apps", "Apps", Monitor],
          ] as const
        ).map(([id, label, Icon]) => (
          <button key={id} role="tab" aria-selected={tab === id} onClick={() => setTab(id)}>
            <Icon size={17} />
            {label}
          </button>
        ))}
      </div>

      {!current.available ? (
        <p className="muted machine-empty">{current.name} is offline. Its agents, folders, and apps show here when it is back.</p>
      ) : (
        <div className="machine-panel" role="tabpanel">
          {problem && <p className="machine-problem">{problem}</p>}

          {tab === "agents" && (
            <>
              {mine.map((id) => props.agentRow(id))}
              {!mine.length && !starting && <p className="muted machine-empty">No agents on {current.name} yet.</p>}
              {starting ? (
                <div className="machine-start">
                  <p>
                    Start an agent on {current.self ? "this computer" : current.name} in <code>{full(path)}</code>.
                    <small> To pick another folder, open it in Files.</small>
                  </p>
                  <div className="provider-list">
                    {providers === null && !problem && <p className="muted">Finding installed agents…</p>}
                    {providers?.length === 0 && <p className="muted">No agent CLIs are installed there.</p>}
                    {providers?.map((p) => (
                      <button
                        key={p.id}
                        className="secondary"
                        disabled={props.busy}
                        onClick={() => props.run(() => props.onStart(current, p.id, full(path)))}
                      >
                        <Terminal size={17} /> {p.name}
                      </button>
                    ))}
                  </div>
                  <button className="secondary" onClick={() => setStarting(false)}>
                    Cancel
                  </button>
                </div>
              ) : (
                <button className="secondary machine-add" onClick={() => setStarting(true)}>
                  <Plus size={17} /> Start an agent here
                </button>
              )}
            </>
          )}

          {tab === "files" && (
            <>
              <div className="filepath">
                <button aria-label="Parent folder" disabled={path === "."} onClick={() => open(path.split("/").slice(0, -1).join("/") || ".")}>
                  <ArrowLeft size={18} />
                </button>
                <code>{full(path)}</code>
              </div>
              <div className="file-toolbar">
                <button
                  className="secondary"
                  onClick={() => {
                    setTab("agents");
                    setStarting(true);
                  }}
                >
                  Start agent in this folder
                </button>
              </div>
              <div className="filelist">
                {entries === null && !problem && <p className="muted">Opening {full(path)}…</p>}
                {entries?.map((e) => (
                  <button key={e.path} onClick={() => (e.directory ? open(e.path) : props.onPreview(current, e.path))}>
                    {e.directory ? <Folder size={20} /> : <FileText size={20} />}
                    <span>{e.name}</span>
                    <small>{e.directory ? "Folder" : Math.ceil(e.size / 1024) + " KB"}</small>
                  </button>
                ))}
                {entries?.length === 0 && <p>This folder is empty.</p>}
              </div>
            </>
          )}

          {tab === "apps" && (
            <>
              <label className="search">
                <Search size={18} />
                <input
                  placeholder={"Search apps on " + current.name}
                  aria-label={"Search apps on " + current.name}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </label>
              {apps === null && !problem && <p className="muted">Finding apps on {current.name}…</p>}
              <div className="app-grid">
                {apps
                  ?.filter((a) => a.name.toLowerCase().includes(search.toLowerCase()))
                  .sort((a, b) => a.name.localeCompare(b.name))
                  .map((a) => (
                    <button key={a.id} title={"Open " + a.name + " on " + current.name} disabled={props.busy} onClick={() => launchApp(a.id)}>
                      <AppTile name={a.name} />
                      <span>{a.name}</span>
                    </button>
                  ))}
              </div>
              {apps && !apps.some((a) => a.name.toLowerCase().includes(search.toLowerCase())) && (
                <p className="muted">{search ? `No apps match “${search}”.` : "No desktop apps found."}</p>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}
