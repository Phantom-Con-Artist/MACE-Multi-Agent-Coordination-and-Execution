import {
  lazy,
  Suspense,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import PublicSite from "./public-site";
import { Button } from "@/components/ui/button";
const AgentMessage = lazy(() => import("./agent-message"));
import {
  MessageCircle,
  Home,
  Folder,
  Activity,
  Settings,
  Plus,
  Search,
  ArrowLeft,
  Send,
  Paperclip,
  X,
  Sun,
  Moon,
  Square,
  Users,
  Terminal,
  MoreVertical,
  ChevronRight,
  Monitor,
  BookOpen,
  Download,
  Network,
  Play,
  Power,
  Trash2,
  Pause,
  FileText,
  WifiOff,
  ArrowDown,
  Pencil,
  ShieldCheck,
} from "lucide-react";
type Agent = {
  id: string;
  name: string;
  provider?: string;
  status: string;
  working_dir: string;
  labels?: Record<string, string>;
};
type Room = { id: string; name: string; members: string[]; roles?:Record<string,string> };
type Msg = {
  id: string;
  sender: string;
  text: string;
  file?: string;
  at: string;
  delivery?: Record<string, string>;
};
type Entry = { name: string; path: string; directory: boolean; size: number };
type Provider = { id: string; name: string; available: boolean };
type Peer = { id: string; name: string; url: string };
type Policy = { mode: string; goal: string; limit: number };
type Run = { id: string; status: string; replies: number; error?: string; active_member?:string };
type Audit = {
  id: number;
  actor: string;
  action: string;
  target: string;
  details: unknown;
  at: string;
};
type Transfer = {
  id: string;
  source: string;
  destination: string;
  bytes: number;
  status: string;
};
async function api<T>(path: string, body?: unknown): Promise<T> {
  const r = await fetch("/api" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!r.ok) throw Error((await r.text()).trim());
  return r.json();
}
type Device = { id: string; name: string; last_seen: string; current?: boolean };
// randomUUID needs a secure context, and LAN sign-in links are plain HTTP.
const randomID = () =>
  Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) =>
    b.toString(16).padStart(2, "0"),
  ).join("");
const stored = (key: string) => {
  try {
    return localStorage.getItem(key) || "";
  } catch {
    return "";
  }
};
const store = (key: string, value: string) => {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* Private windows can refuse storage; the page still works without it. */
  }
};
const deviceID = stored("mace-device") || randomID();
store("mace-device", deviceID);
const full = (p: string) => (p === "." ? "/" : "/" + p.replace(/^\/+/, ""));
const clock = (s: string) =>
  new Date(s).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
const readableError=(error:unknown)=>{const message=(error as Error).message||String(error);if(/dial tcp|connection refused|no such host|i\/o timeout/.test(message))return 'Could not reach that host. Check its address and make sure Mace is running there.';if(/Failed to fetch|NetworkError|Load failed/.test(message))return 'The host is unavailable. Your draft is kept. Check the connection and try again.';if(/^unauthorized$/i.test(message.trim()))return 'That access token was not accepted. Use the token printed by the running host.';return message};
const statusLabel = (status: string) =>
  (
    ({
      ready: "Ready",
      replying: "Working",
      stopped: "Stopped",
      terminated: "Terminated",
      running: "Working",
      paused: "Paused",
      interrupted: "Interrupted",
      failed: "Needs attention",
      completed: "Done",
      limit_reached: "Reply limit reached",
      goal_reported_complete: "Goal reported complete",
      group: "Group",
    }) as Record<string, string>
  )[status] || status.replaceAll("_", " ");
const folderName = (path: string) =>
  path === "/" ? "Root" : path.split("/").filter(Boolean).at(-1) || path;
function Dialog({
  title,
  close,
  children,
}: {
  title: string;
  close: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleID=useId();
  useEffect(() => {
    const dialog=ref.current;if(!dialog)return;if(!dialog.open)dialog.showModal();else{const first=dialog.querySelector<HTMLInputElement>('input:not([type=checkbox]),textarea,select,button');first?.focus()}
  }, [title]);
  return (
    <dialog ref={ref} className="dialog native-dialog" onCancel={close} aria-labelledby={titleID} onKeyDown={event=>{if(event.key!=='Tab')return;const dialog=event.currentTarget;const controls=[...dialog.querySelectorAll<HTMLElement>('button:not([disabled]),a[href],input:not([disabled]),textarea:not([disabled]),select:not([disabled]),[tabindex="0"]')].filter(element=>element.getBoundingClientRect().width>0);const first=controls[0],last=controls.at(-1);if(event.shiftKey&&(document.activeElement===first||!dialog.contains(document.activeElement))){event.preventDefault();last?.focus()}else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}}}>
      <header>
        <h2 id={titleID}>{title}</h2>
        <button aria-label="Close dialog" onClick={close}>
          <X />
        </button>
      </header>
      {children}
    </dialog>
  );
}
export default function Workspace() {
  const initial = location.hash.slice(1);
  const [page, setPage] = useState(
      initial.startsWith("connect=") ? "messenger" : initial || "home",
    ),
    [dark, setDark] = useState(localStorage.getItem("mace-theme") === "dark"),
    [connected, setConnected] = useState(false),
    [online, setOnline] = useState(true),
    [checking, setChecking] = useState(true),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState(""),
    [awayAt, setAwayAt] = useState(""),
    [devices, setDevices] = useState<Device[]>([]);
  // One ID per unsent draft, so a retry after a dropped response is not sent twice.
  const pendingSends = useRef<Record<string, string>>({});
  const [agents, setAgents] = useState<Agent[]>([]),
    [processes, setProcesses] = useState<Agent[]>([]),
    [rooms, setRooms] = useState<Room[]>([]),
    [providers, setProviders] = useState<Provider[]>([]),
    [peers, setPeers] = useState<Peer[]>([]),
    [remote, setRemote] = useState<Agent[]>([]),
    [runs, setRuns] = useState<Run[]>([]),
    [node, setNode] = useState<Record<string, unknown>>({});
  const [selected, select] = useState(""),
    [messages, setMessages] = useState<Msg[]>([]),
    [drafts, setDrafts] = useState<Record<string, string>>(() => {
      try {
        return JSON.parse(sessionStorage.getItem("mace-drafts") || "{}");
      } catch {
        return {};
      }
    }),
    [searches, setSearches] = useState<Record<string, string>>({}),
    [filter, setFilter] = useState("all"),
    [modal, setModal] = useState(""),
    [attachments, setAttachments] = useState<Record<string, string>>({}),
    [groupMembers, setGroupMembers] = useState<string[]>([]),
    [unread, setUnread] = useState(false),
    [roleMember,setRoleMember]=useState(''),[roleDraft,setRoleDraft]=useState(''),
    [policy, setPolicy] = useState<Policy>({
      mode: "reply",
      goal: "",
      limit: 1,
    });
  const [entries, setEntries] = useState<Entry[]>([]),
    [path, setPath] = useState("."),
    [directory, setDirectory] = useState("/"),
    [purpose, setPurpose] = useState("page"),
    [preview, setPreview] = useState(""),
    [activity, setActivity] = useState<Audit[]>([]),
    [transfers, setTransfers] = useState<Transfer[]>([]),
    [apps, setApps] = useState<{ id: string; name: string }[]>([]);
  const [activityLoading, setActivityLoading] = useState(true),
    [appsLoading, setAppsLoading] = useState(false);
  const bottom = useRef<HTMLDivElement>(null);
  const messagePane = useRef<HTMLDivElement>(null),
    composer = useRef<HTMLTextAreaElement>(null),
    nearBottom = useRef(true),
    busyNow = useRef(false);
  const search = searches[page] || "";
  const setSearch = (value: string) =>
    setSearches((previous) => ({ ...previous, [page]: value }));
  const attachment = attachments[selected] || "";
  const setAttachment = (value: string) =>
    setAttachments((previous) => ({ ...previous, [selected]: value }));
  useEffect(() => {
    try {
      sessionStorage.setItem("mace-drafts", JSON.stringify(drafts));
    } catch {}
  }, [drafts]);
  useEffect(() => {
    if (!notice) return;
    const timer = setTimeout(() => setNotice(""), 4000);
    return () => clearTimeout(timer);
  }, [notice]);
  const all = [...agents, ...remote],
    chats = [
      ...rooms.map((r) => ({ ...r, status: "group", working_dir: "" })),
      ...all.filter((a) => a.status !== "removed"),
    ],
    chat = chats.find((c) => c.id === selected),
    room = rooms.find((r) => r.id === selected),
    run = runs.find((r) => r.id === selected),
    running = run?.status === "running";
  const name = (id: string) =>
    id === "you" ? "You" : id==='mace'?'Mace':all.find((a) => a.id === id)?.name || id;
  const nav = (p: string) => {
    location.hash = p;
    setPage(p);
  };
  async function action(fn: () => Promise<void>) {
    if (busyNow.current) return;
    busyNow.current = true;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError(readableError(e));
    } finally {
      busyNow.current = false;
      setBusy(false);
    }
  }
  async function refresh() {
    try {
      const session = await api<{ connected: boolean }>("/session");
      if (!session.connected) {
        setConnected(false);
        setOnline(true);
        return;
      }
      const [a, p, r, c, n, h, ru] = await Promise.all([
        api<Agent[]>("/v1/sessions"),
        api<Agent[]>("/v1/agents/"),
        api<Room[]>("/v1/rooms"),
        api<{ providers: Provider[] }>("/v1/catalog"),
        api<Record<string, unknown>>("/v1/node"),
        api<Peer[]>("/v1/peers"),
        api<Run[]>("/v1/runs"),
      ]);
      setAgents(a);
      setProcesses(p);
      setRooms(r);
      setProviders(c.providers);
      setNode(n);
      setPeers(h);
      setRuns(ru);
      setConnected(true);
      setOnline(true);
    } catch {
      setOnline(false);
    }
  }
  useEffect(() => {
    let alive = true;
    async function init() {
      try {
        if (initial.startsWith("connect=")) {
          await api("/session", {
            token: decodeURIComponent(initial.slice(8)),
          });
          history.replaceState(null, "", "#messenger");
        }
        if (alive) await refresh();
      } catch (e) {
        setError((e as Error).message);
      } finally {
        if (alive) setChecking(false);
      }
    }
    void init();
    const id = setInterval(() => void refresh(), 3000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, []);
  // Presence: the host stops agent work when every signed-in device goes quiet.
  useEffect(() => {
    if (!connected) return;
    let alive = true;
    async function ping() {
      try {
        const r = await api<{ stopped_at?: string }>("/v1/presence", {
          device: deviceID,
        });
        if (alive && r.stopped_at && r.stopped_at !== stored("mace-away-seen"))
          setAwayAt(r.stopped_at);
      } catch {
        /* The connection banner already reports an unreachable host. */
      }
    }
    void ping();
    const id = setInterval(() => void ping(), 5000);
    const leave = () =>
      navigator.sendBeacon(
        "/api/v1/presence/leave",
        JSON.stringify({ device: deviceID }),
      );
    addEventListener("pagehide", leave);
    return () => {
      alive = false;
      clearInterval(id);
      removeEventListener("pagehide", leave);
    };
  }, [connected]);
  useEffect(() => {
    if (connected && page === "settings")
      void api<Device[]>("/v1/devices")
        .then(setDevices)
        .catch(() => setDevices([]));
  }, [connected, page]);
  useEffect(() => {
    const viewport = window.visualViewport;
    const resize = () => {
      document.documentElement.style.setProperty(
        "--viewport-height",
        `${viewport?.height || window.innerHeight}px`,
      );
    };
    resize();
    viewport?.addEventListener("resize", resize);
    window.addEventListener("resize", resize);
    return () => {
      viewport?.removeEventListener("resize", resize);
      window.removeEventListener("resize", resize);
    };
  }, []);
  useEffect(() => {
    const field = composer.current;
    if (!field) return;
    field.style.height = "auto";
    field.style.height = `${Math.min(field.scrollHeight, Math.max(88, Math.min(160, window.innerHeight * 0.24)))}px`;
  }, [drafts, selected, page]);
  useEffect(() => {
    if (modal === "group") setGroupMembers([]);
  }, [modal]);
  useEffect(() => {
    if (connected) void import("./agent-message");
  }, [connected]);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    localStorage.setItem("mace-theme", dark ? "dark" : "light");
  }, [dark]);
  useEffect(() => {
    const fn = () => setPage(location.hash.slice(1) || "home");
    window.addEventListener("hashchange", fn);
    return () => window.removeEventListener("hashchange", fn);
  }, []);
  useEffect(() => {
    let alive = true;
    async function load() {
      const results = await Promise.allSettled(
        peers.map((p) => api<Agent[]>("/v1/peers/" + p.id + "/sessions")),
      );
      if (alive) {
        setRemote(
          results.flatMap((r) => (r.status === "fulfilled" ? r.value : [])),
        );
        const failed = results.flatMap((r, i) =>
          r.status === "rejected" ? [peers[i].name] : [],
        );
        if (failed.length) setError("Hosts unavailable: " + failed.join(", "));
      }
    }
    void load();
    const id = setInterval(() => void load(), 10000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [peers.map((p) => p.id).join(",")]);
  useEffect(() => {
    if (!selected) return;
    let alive = true;
    setMessages([]);
    async function load() {
      try {
        const m = await api<Msg[]>(
          "/v1/chats/" + encodeURIComponent(selected) + "/messages",
        );
        if (alive) setMessages(m);
      } catch {
        if (alive) setOnline(false);
      }
    }
    void load();
    const id = setInterval(() => void load(), 1000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [selected]);
  useEffect(() => {
    if (selected)
      void api<Policy>("/v1/chats/" + encodeURIComponent(selected) + "/policy")
        .then(setPolicy)
        .catch(() => {});
  }, [selected]);
  useEffect(() => {
    if (nearBottom.current) {
      bottom.current?.scrollIntoView({ block: "end" });
      setUnread(false);
    } else {
      setUnread(true);
    }
  }, [messages.length, selected]);
  useEffect(() => {
    nearBottom.current = true;
    setUnread(false);
  }, [selected]);
  useEffect(() => {
    if (!connected || !["activity", "host"].includes(page)) return;
    let alive = true;
    setActivityLoading(true);
    async function load() {
      try {
        const [a, t] = await Promise.all([
          api<Audit[]>("/v1/activity"),
          api<Transfer[]>("/v1/transfers"),
        ]);
        if (alive) {
          setActivity(a);
          setTransfers(t);
        }
      } catch (e) {
        if (alive) setError((e as Error).message);
      } finally {
        if (alive) setActivityLoading(false);
      }
    }
    void load();
    const id = setInterval(() => void load(), 3000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [page, connected]);
  useEffect(() => {
    if (page !== "apps" || !connected) return;
    let alive = true;
    setAppsLoading(true);
    void api<typeof apps>("/v1/apps")
      .then((list) => {
        if (alive) setApps(list);
      })
      .catch((e) => {
        if (alive) setError((e as Error).message);
      })
      .finally(() => {
        if (alive) setAppsLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [page, connected]);
  async function browse(p: string, why = purpose) {
    await action(async () => {
      setEntries(
        (await api<Entry[]>("/v1/files?path=" + encodeURIComponent(p))).sort(
          (a, b) =>
            Number(b.directory) - Number(a.directory) ||
            a.name.localeCompare(b.name),
        ),
      );
      setPath(p === "/" ? "." : p);
      setPurpose(why);
      if (why !== "page") setModal("files");
    });
  }
  async function launch(provider: string) {
    await action(async () => {
      const a = await api<Agent>("/v1/sessions", {
        provider,
        working_dir: directory,
      });
      await refresh();
      select(a.id);
      setModal("");
      nav("messenger");
    });
  }
  async function control(id: string, operation: string) {
    await action(async () => {
      const updated = await api<Agent>("/v1/sessions/" + id + "/control", {
        action: operation,
      });
      if (id.includes("::"))
        setRemote((previous) =>
          previous.map((agent) =>
            agent.id === id ? { ...updated, id, name: agent.name } : agent,
          ),
        );
      await refresh();
      if (operation === "remove") {
        select("");
        setModal("");
      }
      setNotice(
        (
          {
            stop: "Agent stopped",
            terminate: "Agent terminated",
            resume: "Agent ready",
            remove: "Agent removed",
          } as Record<string, string>
        )[operation],
      );
    });
  }
  async function open(app?: string, p?: string) {
    await action(async () => {
      await api("/v1/host/open", { app, path: p });
      await refresh();
      setNotice(
        "Launch requested on the host. Process results are in Activity.",
      );
    });
  }
  async function discoverApps() {
    await action(async () => {
      setApps(await api<typeof apps>("/v1/apps"));
      nav("apps");
    });
  }
  const files = (
    <>
      <div className="filepath">
        <button
          aria-label="Parent folder"
          onClick={() =>
            void browse(path.split("/").slice(0, -1).join("/") || ".")
          }
        >
          <ArrowLeft size={18} />
        </button>
        <code>{full(path)}</code>
      </div>
      <div className="filelist">
        {entries.map((e) => (
          <button
            key={e.path}
            onClick={() => {
              if (e.directory) {
                void browse(e.path);
                return;
              }
              if (purpose === "folder") return;
              if (purpose === "attach") {
                setAttachment(e.path);
                setModal("");
              } else {
                setPreview(e.path);
                setModal("preview");
              }
            }}
          >
            {e.directory ? <Folder size={20} /> : <FileText size={20} />}
            <span>{e.name}</span>
            <small>
              {e.directory ? "Folder" : Math.ceil(e.size / 1024) + " KB"}
            </small>
          </button>
        ))}
        {!entries.length && <p>This folder is empty.</p>}
      </div>
      {purpose === "folder" && (
        <button
          className="primary"
          onClick={() => {
            setDirectory(full(path));
            setModal("launch");
          }}
        >
          Use this folder
        </button>
      )}
    </>
  );
  const picker = (
    <>
      <p>Choose an installed agent. Start chatting.</p>
      <button
        className="folder-choice"
        onClick={() => void browse(directory, "folder")}
      >
        <Folder size={20} />
        <span>
          <small>Start in</small>
          <strong>{directory === "/" ? "Root /" : directory}</strong>
        </span>
        <span>
          Change <ChevronRight size={16} />
        </span>
      </button>
      <div className="provider-list">
        {providers
          .filter((p) => p.available)
          .map((p) => (
            <Button
              variant="ghost"
              disabled={busy}
              key={p.id}
              onClick={() => void launch(p.id)}
            >
              <div className="avatar">
                <Terminal size={22} />
              </div>
              <span>
                <strong>{p.name}</strong>
                <small>Installed on this host</small>
              </span>
              <Plus size={21} />
            </Button>
          ))}
      </div>
      {!providers.some((p) => p.available) && (
        <p>
          No supported agents found. Install a supported CLI on the host and
          refresh.
        </p>
      )}
      <details>
        <summary>Agent availability</summary>
        {providers.map((p) => (
          <p key={p.id}>
            {p.name}: {p.available ? "installed" : "not found"}
          </p>
        ))}
      </details>
    </>
  );
  const navItems: [string, typeof Home, string][] = [
    ["messenger", MessageCircle, "Chats"],
    ["host", Monitor, "Host"],
    ["activity", Activity, "Activity"],
    ["settings", Settings, "Settings"],
    ["more", MoreVertical, "More"],
  ];
  if (["home", "manual", "docs", "troubleshoot", "about"].includes(page))
    return (
      <PublicSite
        page={page}
        navigate={nav}
        dark={dark}
        toggleTheme={() => setDark(!dark)}
        connected={connected}
      />
    );
  return (
    <div className="app">
      <nav className="rail" aria-label="Main navigation">
        <a className="brand" href="#home" aria-label="Mace home">
          <img src="/mace-mark.svg" alt="Mace" />
        </a>
        <div className="navlinks">
          {navItems.map(([id, Icon, label]) => (
            <button
              key={id}
              aria-label={label}
              title={label}
              aria-current={page === id ? "page" : undefined}
              className={page === id ? "active" : ""}
              onClick={() => (id === "more" ? setModal("more") : nav(id))}
            >
              <Icon size={22} />
              <span>{label}</span>
            </button>
          ))}
        </div>
        <button
          className="theme"
          aria-label="Toggle theme"
          onClick={() => setDark(!dark)}
        >
          {dark ? <Sun /> : <Moon />}
        </button>
      </nav>
      <main>
        {connected && !online && (
          <div className="connection-banner" role="status">
            <WifiOff size={18} />
            <span>Connection lost. Your draft is safe. Reconnecting…</span>
            <button onClick={() => void refresh()}>Retry</button>
          </div>
        )}
        {connected && awayAt && (
          <div className="connection-banner away-banner" role="status">
            <ShieldCheck size={18} />
            <span>
              While you were away, agents were stopped at {clock(awayAt)}.
              Open a conversation and choose Continue to resume.
            </span>
            <button
              onClick={() => {
                store("mace-away-seen", awayAt);
                setAwayAt("");
              }}
            >
              Dismiss
            </button>
          </div>
        )}
        {connected && runs.some((r) => r.status === "running") && (
          <div className="connection-banner work-banner" role="status">
            <Activity size={18} />
            <span>
              {runs.filter((r) => r.status === "running").length === 1
                ? "1 conversation is working."
                : `${runs.filter((r) => r.status === "running").length} conversations are working.`}
            </span>
            <button
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  await api("/v1/pause-all", {});
                  await refresh();
                  setNotice("Everything is paused. Continue each conversation when ready.");
                })
              }
            >
              Pause everything
            </button>
          </div>
        )}
        {!connected ? (
          <div className="connect-screen">
            <img src="/mace-mark.svg" width="64" height="64" alt="Mace" />
            <h1>
              {checking
                ? "Connecting…"
                : !online
                  ? "Your host is unavailable."
                  : "Your host is one click away."}
            </h1>
            <p>
              Open the sign-in link printed by <code>mace start</code>.
            </p>
            {!checking && !online && (
              <button className="primary" onClick={() => void refresh()}>
                Try again
              </button>
            )}
            {!checking && online && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  const token = new FormData(e.currentTarget).get("token");
                  void action(async () => {
                    await api("/session", { token });
                    await refresh();
                  });
                }}
              >
                <label>
                  Or paste your access token
                  <input
                    name="token"
                    type="password"
                    autoComplete="current-password"
                    required
                  />
                </label>
                <button className="primary" disabled={busy}>
                  Connect
                </button>
              </form>
            )}
          </div>
        ) : page === "messenger" ? (
          <div className={"messenger " + (selected ? "has-chat" : "")}>
            <aside className="chatlist">
              <header>
                <h1>
                  Chats<span className="count">{chats.length}</span>
                </h1>
                <div>
                  <button
                    aria-label="New group"
                    onClick={() => setModal("group")}
                  >
                    <Users size={21} />
                  </button>
                  <button
                    aria-label="New agent"
                    onClick={() => setModal("launch")}
                  >
                    <Plus size={24} />
                  </button>
                </div>
              </header>
              <label className="search">
                <Search size={18} />
                <input
                  aria-label="Search conversations"
                  placeholder="Search conversations"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </label>
              <div className="filters">
                {["all", "agents", "groups"].map((f) => (
                  <button
                    className={filter === f ? "chosen" : ""}
                    key={f}
                    onClick={() => setFilter(f)}
                  >
                    {f[0].toUpperCase() + f.slice(1)}
                  </button>
                ))}
              </div>
              <div className="conversations">
                {chats
                  .filter(
                    (c) =>
                      c.name.toLowerCase().includes(search.toLowerCase()) &&
                      (filter === "all" ||
                        (filter === "groups"
                          ? c.status === "group"
                          : c.status !== "group")),
                  )
                  .map((c) => (
                    <button
                      className={
                        "conversation " + (selected === c.id ? "selected" : "")
                      }
                      key={c.id}
                      onClick={() => {
                        select(c.id);
                      }}
                    >
                      <div className="avatar">
                        {c.status === "group" ? <Users /> : <Terminal />}
                      </div>
                      <div className="chatcopy">
                        <div>
                          <strong>{c.name}</strong>
                          <small>{statusLabel(c.status)}</small>
                        </div>
                        <p>
                          {c.status === "group"
                            ? `${rooms.find((r) => r.id === c.id)?.members.length} agents`
                            : folderName(c.working_dir)}
                        </p>
                      </div>
                    </button>
                  ))}
                {!chats.length && (
                  <div className="emptylist">
                    <MessageCircle size={34} />
                    <h2>Who shall we work with?</h2>
                    <p>Choose from your installed agents.</p>
                    <button
                      className="primary"
                      onClick={() => setModal("launch")}
                    >
                      Choose an agent <Plus size={17} />
                    </button>
                  </div>
                )}
              </div>
              <footer className="host">
                <span className={online ? "online" : "offline"} />
                <div>
                  <strong>
                    {String(node.host_name || node.name || "Your host")}
                  </strong>
                  <small>
                    {online
                      ? "Connected · private workspace"
                      : "Reconnecting to host"}
                  </small>
                </div>
                <button aria-label="Open host" onClick={() => nav("host")}>
                  <ChevronRight size={18} />
                </button>
              </footer>
            </aside>
            <section className="thread" aria-label="Conversation">
              {chat ? (
                <>
                  <header className="threadhead">
                    <button
                      className="back"
                      aria-label="Back to chats"
                      onClick={() => select("")}
                    >
                      <ArrowLeft />
                    </button>
                    <div className="avatar">
                      {room ? <Users /> : <Terminal />}
                    </div>
                    <div className="threadtitle">
                      <strong>{chat.name}</strong>
                      <small title={chat.working_dir}>
                        {running
                          ? run.active_member?name(run.active_member)+' is working':'Team conversation is running'
                          : statusLabel(chat.status) +
                            (chat.working_dir
                              ? " · " + folderName(chat.working_dir)
                              : "")}
                      </small>
                    </div>
                    <button
                      aria-label={running?'Pause conversation':room&&policy.mode==='continuous'?'Start conversation':'Pause conversation'}
                      disabled={busy||(!running&&(!room||policy.mode!=='continuous'||!room.members.length))}
                      onClick={() =>
                        void action(async () => {
                          await api(
                            "/v1/chats/" +
                              encodeURIComponent(selected) +
                              (running?"/pause":"/start"),
                            {},
                          );
                          await refresh();
                        })
                      }
                    >
                      {!running&&room&&policy.mode==='continuous'?<Play size={20}/>:<Pause size={20}/>}
                    </button>
                    <button
                      aria-label="Conversation controls"
                      onClick={() => setModal("details")}
                    >
                      <MoreVertical />
                    </button>
                  </header>
                  <button
                    className="policy-strip"
                    onClick={() => setModal("policy")}
                  >
                    <span>
                      {policy.mode === "goal"
                        ? "Goal: " + policy.goal
                        : policy.mode === "continuous"
                          ? "Continuous · until paused"
                          : "Reply limit · " +
                            policy.limit +
                            (policy.limit === 1 ? " reply" : " replies")}
                    </span>
                    <span>
                      Change <ChevronRight size={14} />
                    </span>
                  </button>
                  <div
                    className="messages"
                    ref={messagePane}
                    onScroll={(e) => {
                      const pane = e.currentTarget;
                      nearBottom.current =
                        pane.scrollHeight - pane.scrollTop - pane.clientHeight <
                        96;
                      if (nearBottom.current) setUnread(false);
                    }}
                  >
                    {!messages.length && (
                      <div className="startnote">
                        What would you like {chat.name} to do?
                      </div>
                    )}
                    {messages.map((m) => (
                      <article
                        key={m.id}
                        className={
                          (m.sender==='mace'?'system-message ':'bubble ') +
                          (m.sender === "you" ? "outgoing" : "incoming")
                        }
                      >
                        <strong
                          className={
                            "sender" +
                            (m.sender === "you" || m.sender==='mace' ||
                            (!room && m.sender === selected)
                              ? " sr-only"
                              : "")
                          }
                        >
                          {name(m.sender)}
                        </strong>
                        <div
                          className={
                            "message-text " +
                            (m.sender === "you" ? "plain" : "markdown")
                          }
                        >
                          {m.sender === "you" ? (
                            m.text
                          ) : (
                            <Suspense fallback={<span>{m.text}</span>}>
                              <AgentMessage text={m.text} />
                            </Suspense>
                          )}
                        </div>
                        {m.file && <Media path={m.file} />}
                        <small>
                          {Object.values(m.delivery || {}).some(
                            (v) => v !== "delivered",
                          )
                            ? "Delivery issue · "
                            : ""}
                          {clock(m.at)}
                        </small>
                      </article>
                    ))}
                    {running && (
                      <div className="work-state" role="status">
                        Agents are working. You can pause this conversation.
                      </div>
                    )}
                    {run?.status === "interrupted" ? (
                      <div className="run-interrupted" role="status">
                        {/* The reason is already in the conversation as a note. */}
                        <p>Work stopped here. Continue to pick up where it left off.</p>
                        <button
                          className="primary"
                          disabled={busy || !online}
                          onClick={() =>
                            void action(async () => {
                              await api(
                                "/v1/chats/" +
                                  encodeURIComponent(selected) +
                                  "/continue",
                                {},
                              );
                              if (awayAt) store("mace-away-seen", awayAt);
                              setAwayAt("");
                              await refresh();
                            })
                          }
                        >
                          <Play size={16} />
                          Continue
                        </button>
                      </div>
                    ) : run?.error && (
                      <div className="run-error" role="alert">
                        {run.error}
                        <button onClick={() => nav("activity")}>
                          View activity
                        </button>
                      </div>
                    )}
                    {run && !running && run.status !== "interrupted" && (
                      <div className="work-state">
                        {statusLabel(run.status)} · {run.replies}{" "}
                        {run.replies === 1 ? "reply" : "replies"}
                      </div>
                    )}
                    <div ref={bottom} />
                  </div>
                  {unread && (
                    <button
                      className="new-messages"
                      onClick={() => {
                        nearBottom.current = true;
                        bottom.current?.scrollIntoView({ block: "end" });
                        setUnread(false);
                      }}
                    >
                      <ArrowDown size={16} />
                      New messages
                    </button>
                  )}
                  {attachment && (
                    <div className="attachment">
                      {attachment}
                      <button
                        aria-label="Remove attachment"
                        onClick={() => setAttachment("")}
                      >
                        <X size={16} />
                      </button>
                    </div>
                  )}
                  <form
                    className="composer"
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (
                        running ||
                        busyNow.current ||
                        (!(drafts[selected] || "").trim() && !attachment)
                      )
                        return;
                      void action(async () => {
                        const conversation = selected;
                        pendingSends.current[conversation] ||= randomID();
                        const m = await api<Msg>(
                          "/v1/chats/" +
                            encodeURIComponent(conversation) +
                            "/messages",
                          {
                            text: drafts[conversation] || "",
                            file: attachment,
                            client_id: pendingSends.current[conversation],
                          },
                        );
                        delete pendingSends.current[conversation];
                        if (
                          Object.values(m.delivery || {}).some(
                            (value) => value !== "delivered",
                          )
                        ) {
                          setMessages((v) => [...v, m]);
                          throw Error(
                            "Your message was saved but could not be delivered. Your draft is kept so you can try again.",
                          );
                        }
                        nearBottom.current = true;
                        setMessages((v) => [...v, m]);
                        setDrafts((v) => ({ ...v, [selected]: "" }));
                        setAttachment("");
                        await refresh();
                      });
                    }}
                  >
                    <button
                      type="button"
                      aria-label="Attach host file"
                      onClick={() => void browse(".", "attach")}
                    >
                      <Paperclip size={23} />
                    </button>
                    <textarea
                      ref={composer}
                      aria-label="Message"
                      aria-describedby="composer-hint"
                      autoCapitalize="sentences"
                      placeholder={
                        running
                          ? "Agents are working…"
                          : "Tell your agents what to do"
                      }
                      rows={1}
                      value={drafts[selected] || ""}
                      onChange={(e) => {
                        const value = e.currentTarget.value;
                        const conversation = selected;
                        setDrafts((v) => ({ ...v, [conversation]: value }));
                      }}
                      onKeyDown={(e) => {
                        if (e.nativeEvent.isComposing || e.keyCode === 229)
                          return;
                        if (
                          e.key === "Enter" &&
                          (e.ctrlKey ||
                            e.metaKey ||
                            (!e.shiftKey &&
                              window.matchMedia(
                                "(hover: hover) and (pointer: fine)",
                              ).matches))
                        ) {
                          e.preventDefault();
                          e.currentTarget.form?.requestSubmit();
                        }
                      }}
                    />
                    <button
                      className="send"
                      aria-label="Send message"
                      disabled={
                        busy ||
                        !online ||
                        running ||
                        (!(drafts[selected] || "").trim() && !attachment)
                      }
                    >
                      <Send size={22} />
                    </button>
                  </form>
                  <span id="composer-hint" className="sr-only">
                    On a phone, Enter adds a new line. Use Send or Control Enter
                    to send. On a desktop, Shift Enter adds a new line.
                  </span>
                </>
              ) : (
                <div className="welcome">
                  <img src="/mace-mark.svg" width="90" height="90" alt="" />
                  <h2>
                    Your agents.
                    <br />
                    Your machine.
                  </h2>
                  <p>Choose an agent and tell it what to do.</p>
                  <button
                    className="primary"
                    onClick={() => setModal("launch")}
                  >
                    Start a chat <Plus size={18} />
                  </button>
                  <small>Multi Agent Coordination and Execution</small>
                </div>
              )}
            </section>
          </div>
        ) : (
          <div className="page">
            <div className="page-top">
              <span>MACE / {page}</span>
              <button
                aria-label="Open messenger"
                onClick={() => nav("messenger")}
              >
                <MessageCircle size={20} />
              </button>
            </div>
            {page === "host" ? (
              <>
                <h1>Your host.</h1>
                <p className="lede">
                  {String(node.host_name || node.name)} · {String(node.os)} ·{" "}
                  {String(node.arch)}
                </p>
                <div className="host-actions">
                  <button
                    onClick={() => {
                      nav("files");
                      void browse(".", "page");
                    }}
                  >
                    <Folder />
                    <span>
                      <strong>Browse files</strong>
                      <small>Open any accessible folder</small>
                    </span>
                    <ChevronRight />
                  </button>
                  <button onClick={() => void discoverApps()}>
                    <Monitor />
                    <span>
                      <strong>Open an app</strong>
                      <small>Launch on the host desktop</small>
                    </span>
                    <ChevronRight />
                  </button>
                  <button onClick={() => setModal("launch")}>
                    <Terminal />
                    <span>
                      <strong>Start an agent</strong>
                      <small>Choose an installed CLI</small>
                    </span>
                    <ChevronRight />
                  </button>
                  <button onClick={() => nav("hosts")}>
                    <Network />
                    <span>
                      <strong>Connected hosts</strong>
                      <small>{peers.length} linked</small>
                    </span>
                    <ChevronRight />
                  </button>
                </div>
                <h2>Agent sessions</h2>
                {agents
                  .filter((a) => a.status !== "removed")
                  .map((a) => (
                    <div className="session-row" key={a.id}>
                      <button
                        onClick={() => {
                          select(a.id);
                          nav("messenger");
                        }}
                      >
                        <Terminal size={18} />
                        <span>
                          {a.name}
                          <small>
                            {a.working_dir} · {a.status}
                          </small>
                        </span>
                      </button>
                      <button
                        aria-label={"Stop " + a.name}
                        onClick={() => void control(a.id, "stop")}
                      >
                        <Square size={17} />
                      </button>
                      <button
                        aria-label={"Terminate " + a.name}
                        onClick={() => void control(a.id, "terminate")}
                      >
                        <Power size={17} />
                      </button>
                      <button
                        aria-label={"Remove " + a.name}
                        onClick={() => void control(a.id, "remove")}
                      >
                        <Trash2 size={17} />
                      </button>
                    </div>
                  ))}
                <p className="muted">
                  Desktop apps need an active host desktop session. Check
                  Activity for launch results.
                </p>
              </>
            ) : page === "files" ? (
              <>
                <h1>Host files.</h1>
                <p className="lede">
                  Browse, inspect, download, or open on your host.
                </p>
                <div className="file-toolbar">
                  <button
                    className="secondary"
                    onClick={() => void browse(".", "page")}
                  >
                    Root /
                  </button>
                  <button
                    className="secondary"
                    onClick={() => void open(undefined, full(path))}
                  >
                    Open folder on host
                  </button>
                  <button
                    className="secondary"
                    onClick={() => {
                      setDirectory(full(path));
                      setModal("launch");
                    }}
                  >
                    Start agent here
                  </button>
                </div>
                {files}
              </>
            ) : page === "apps" ? (
              <>
                <h1>Host apps.</h1>
                <p className="lede">
                  Click an app to open it on your host desktop.
                </p>
                <label className="search">
                  <Search size={18} />
                  <input
                    placeholder="Search apps"
                    aria-label="Search host apps"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </label>
                {appsLoading && (
                  <p className="muted" role="status">
                    Finding your host apps…
                  </p>
                )}
                <div className="apps-list">
                  {apps
                    .filter((a) =>
                      a.name.toLowerCase().includes(search.toLowerCase()),
                    )
                    .sort((a, b) => a.name.localeCompare(b.name))
                    .map((a) => (
                      <button
                        key={a.id}
                        title={a.name}
                        disabled={busy}
                        onClick={() => void open(a.id)}
                      >
                        <span>{a.name}</span>
                        <ChevronRight size={17} />
                      </button>
                    ))}
                </div>
                {!appsLoading &&
                  apps.length > 0 &&
                  !apps.some((a) =>
                    a.name.toLowerCase().includes(search.toLowerCase()),
                  ) && (
                    <div className="section-empty">
                      <p>No apps match “{search}”.</p>
                      <button
                        className="secondary"
                        onClick={() => setSearch("")}
                      >
                        Clear search
                      </button>
                    </div>
                  )}
                {!appsLoading && !apps.length && (
                  <button
                    className="primary"
                    onClick={() => void discoverApps()}
                  >
                    Discover apps
                  </button>
                )}
              </>
            ) : page === "activity" ? (
              <>
                <h1>Activity.</h1>
                <p className="lede">
                  Who did what. Native process logs and file transactions.
                </p>
                {activityLoading ? (
                  <div
                    className="activity-loading"
                    role="status"
                    aria-label="Loading activity"
                  >
                    <div />
                    <div />
                    <div />
                  </div>
                ) : !(
                    activity.length ||
                    processes.length ||
                    runs.length ||
                    transfers.length
                  ) ? (
                  <div className="activity-empty">
                    <Activity size={30} />
                    <h2>Nothing has run yet.</h2>
                    <p>
                      Start an agent or open a host app. You will find its work,
                      logs, and file transfers here.
                    </p>
                    <button
                      className="primary"
                      onClick={() => setModal("launch")}
                    >
                      Choose an agent <Plus size={17} />
                    </button>
                  </div>
                ) : (
                  <>
                    <div className="metrics">
                      <div>
                        <strong>
                          {agents.filter((a) => a.status !== "removed").length}
                        </strong>
                        <span>Agent sessions</span>
                      </div>
                      <div>
                        <strong>
                          {runs.filter((r) => r.status === "running").length}
                        </strong>
                        <span>Working conversations</span>
                      </div>
                      <div>
                        <strong>{transfers.length}</strong>
                        <span>Transfers</span>
                      </div>
                    </div>
                    {runs.length > 0 && <h2>Conversations</h2>}
                    {runs.map((r) => (
                      <button
                        className="process-row"
                        key={r.id}
                        onClick={() => {
                          select(r.id);
                          nav("messenger");
                        }}
                      >
                        <span>
                          {chats.find((c) => c.id === r.id)?.name || r.id}
                        </span>
                        <small>
                          {statusLabel(r.status)} · {r.replies}{" "}
                          {r.replies === 1 ? "reply" : "replies"}
                        </small>
                      </button>
                    ))}
                    <h2>Recent activity</h2>
                    <label className="search">
                      <Search size={18} />
                      <input
                        placeholder="Filter events"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                      />
                    </label>
                    <div className="audit-list">
                      {activity
                        .filter((a) =>
                          JSON.stringify(a)
                            .toLowerCase()
                            .includes(search.toLowerCase()),
                        )
                        .map((a) => (
                          <details key={a.id}>
                            <summary>
                              <time>{clock(a.at)}</time>
                              <span>
                                {a.action.replaceAll("_", " ").toLowerCase()}
                              </span>
                              <small>{name(a.actor)}</small>
                            </summary>
                            <pre>
                              {JSON.stringify(
                                { target: a.target, details: a.details },
                                null,
                                2,
                              )}
                            </pre>
                          </details>
                        ))}
                    </div>
                    {transfers.length > 0 && <h2>File transactions</h2>}
                    {transfers.map((t) => (
                      <details key={t.id}>
                        <summary>
                          {t.source} · {t.bytes} bytes · {t.status}
                        </summary>
                        <pre>{JSON.stringify(t, null, 2)}</pre>
                      </details>
                    ))}
                    {processes.length > 0 && <h2>Native process logs</h2>}
                    {processes.map((p) => (
                      <button
                        className="process-row"
                        key={p.id}
                        onClick={() => {
                          select(p.id);
                          setModal("logs");
                        }}
                      >
                        <Terminal size={16} />
                        <span>{p.name}</span>
                        <small>{p.status}</small>
                        <ChevronRight size={16} />
                      </button>
                    ))}
                  </>
                )}
              </>
            ) : page === "hosts" ? (
              <>
                <h1>Connected hosts.</h1>
                <p className="lede">
                  Bring another machine's agents into your groups.
                </p>
                <button className="primary" onClick={() => setModal("peer")}>
                  Connect a host <Plus size={18} />
                </button>
                {peers.map((p) => (
                  <section className="peer-row" key={p.id}>
                    <h2>{p.name}</h2>
                    <p>{p.url}</p>
                    {remote
                      .filter((a) => a.id.startsWith(p.id + "::"))
                      .map((a) => (
                        <button
                          key={a.id}
                          className="secondary"
                          onClick={() => {
                            select(a.id);
                            nav("messenger");
                          }}
                        >
                          <MessageCircle size={18} />
                          {a.name}
                        </button>
                      ))}
                    <p className="muted">
                      Start agents on this host first. They will appear here
                      automatically.
                    </p>
                  </section>
                ))}
              </>
            ) : page === "settings" ? (
              <>
                <h1>Settings.</h1>
                <section className="setting">
                  <div>
                    <h2>Appearance</h2>
                    <p>Choose what feels comfortable.</p>
                  </div>
                  <button className="secondary" onClick={() => setDark(!dark)}>
                    {dark ? <Sun size={18} /> : <Moon size={18} />}Switch to{" "}
                    {dark ? "light" : "dark"}
                  </button>
                </section>
                <section className="setting">
                  <div>
                    <h2>Other machines</h2>
                    <p>Connect using the address and token printed by Mace.</p>
                  </div>
                  <button className="secondary" onClick={() => nav("hosts")}>
                    Manage hosts
                  </button>
                </section>
                <section className="setting devices">
                  <div>
                    <h2>Signed-in devices</h2>
                    <p>
                      Each browser keeps its own sign-in. Remove one you no
                      longer use.
                    </p>
                  </div>
                  <ul>
                    {devices.map((d) => (
                      <li key={d.id}>
                        <span>
                          {d.name}
                          <small>
                            {d.current ? "This device" : "Last seen " + clock(d.last_seen)}
                          </small>
                        </span>
                        <button
                          className="secondary"
                          disabled={busy}
                          onClick={() =>
                            void action(async () => {
                              if (d.current) {
                                await api("/session/sign-out", {});
                                setConnected(false);
                                return;
                              }
                              await api("/v1/devices/" + d.id + "/revoke", {});
                              setDevices((v) => v.filter((x) => x.id !== d.id));
                              setNotice(d.name + " was signed out.");
                            })
                          }
                        >
                          {d.current ? "Sign out" : "Remove"}
                        </button>
                      </li>
                    ))}
                    {!devices.length && (
                      <li className="muted">
                        Signed in with the access token. Browser sign-ins appear
                        here.
                      </li>
                    )}
                  </ul>
                </section>
                <button
                  className="control-row"
                  onClick={() => nav("configuration")}
                >
                  Host configuration <ChevronRight size={17} />
                </button>
              </>
            ) : page === "configuration" ? (
              <>
                <h1>Configuration.</h1>
                <h2>Agent discovery</h2>
                {providers.map((p) => (
                  <p key={p.id}>
                    {p.name}: {p.available ? "installed" : "not found on PATH"}
                  </p>
                ))}
                <h2>Connected node</h2>
                <pre>{JSON.stringify(node, null, 2)}</pre>
                <p className="muted">
                  Credentials and permissions come from each CLI's native host
                  installation.
                </p>
              </>
            ) : page === "manual" ? (
              <>
                <h1>Just start.</h1>
                <p className="lede">
                  Run <code>mace start</code>. Open the sign-in link it prints.
                </p>
                {[
                  [
                    "Start chatting",
                    "Tap +, then choose an installed agent. It starts in root /. Use Change to pick any folder.",
                  ],
                  [
                    "Work together",
                    "Create a group with local or connected-host agents. Reply limit sets a total response budget. Goal mode stops on an agent completion report or the reply budget. Continuous mode runs until paused.",
                  ],
                  [
                    "Stay in control",
                    "Conversation controls let you stop, terminate, resume, and remove agents. Remove group members independently. Pause cancels current work.",
                  ],
                  [
                    "Use your host",
                    "Host → Browse files opens folders and previews media. Start an agent in the folder you are browsing. Open an app launches on the host desktop. Check Activity for process results.",
                  ],
                  [
                    "Connect machines",
                    "Run mace start on another machine, then add its address and access token in Connected hosts. Add its agents to groups. Local agents share paths directly; remote attachments transfer with SHA-256 verification, up to 32 MiB per file.",
                  ],
                  [
                    "Stop Mace",
                    "Use mace stop for graceful shutdown or mace kill to force it. History stays on the host.",
                  ],
                ].map(([h, p]) => (
                  <section className="manual-section" key={h}>
                    <h2>{h}</h2>
                    <p>{p}</p>
                  </section>
                ))}
              </>
            ) : (
              <>
                <div className="homeintro">
                  <img src="/mace-mark.svg" width="56" height="56" alt="" />
                  <h1>
                    Tell your machine.
                    <br />
                    Get on with your day.
                  </h1>
                  <p>
                    Your agents, host apps, and files in one private workspace.
                  </p>
                  <button className="primary" onClick={() => nav("messenger")}>
                    Open chats <ChevronRight size={18} />
                  </button>
                </div>
                <div className="home-row">
                  <h2>Ready when you are.</h2>
                  <div>
                    <h3>Pick an agent</h3>
                    <p>Choose from CLI agents installed on your host.</p>
                    <h3>Bring a team</h3>
                    <p>Group agents from this machine or another Mace host.</p>
                    <h3>Follow the work</h3>
                    <p>
                      Conversations, process logs, and file transactions stay
                      visible in Activity.
                    </p>
                    <a href="#manual">Read the short manual →</a>
                  </div>
                </div>
              </>
            )}
          </div>
        )}
      </main>
      {error && !modal && (
        <div className="error" role="alert">
          <span>{error}</span>
          <button aria-label="Dismiss error" onClick={() => setError("")}>
            <X size={18} />
          </button>
        </div>
      )}
      {notice && (
        <div className="notice" role="status">
          {notice}
          <button aria-label="Dismiss notice" onClick={() => setNotice("")}>
            <X size={16} />
          </button>
        </div>
      )}
      {modal && (
        <Dialog
          title={
            {
              launch: "Choose an agent",
              group: "New group",
              files: purpose === "folder" ? "Choose a folder" : "Host files",
              preview: "File preview",
              details: "Conversation controls",
              policy: "Conversation mode",
              peer: "Connect a host",
              more: "More",
              logs: "Native process log",
              transfer: "Send file",
              role:'Assign a role',
            }[modal] || modal
          }
          close={() => {
            setModal("");
            setError("");
          }}
        >
          {error && (
            <div className="dialog-error" role="alert">
              <span>{error}</span>
              <button
                aria-label="Dismiss dialog error"
                onClick={() => setError("")}
              >
                <X size={18} />
              </button>
            </div>
          )}
          {modal === "launch" ? (
            picker
          ) : modal === "files" ? (
            files
          ) : modal === "preview" ? (
            <>
              <Media path={preview} />
              <button
                className="secondary"
                onClick={() => void open(undefined, full(preview))}
              >
                Open on host
              </button>
              <button
                className="secondary"
                onClick={() => setModal("transfer")}
              >
                Send to another host
              </button>
            </>
          ) : modal === "group" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                const d = new FormData(e.currentTarget);
                if (!groupMembers.length) {
                  setError("Choose at least one agent for this group.");
                  return;
                }
                void action(async () => {
                  const r = await api<Room>("/v1/rooms", {
                    name: d.get("name"),
                    members: groupMembers,
                  });
                  await refresh();
                  select(r.id);
                  setModal("");
                  nav("messenger");
                });
              }}
            >
              <label>
                Group name
                <input
                  name="name"
                  required
                  autoFocus
                  placeholder="What are we working on?"
                />
              </label>
              <fieldset>
                <legend>Choose agents</legend>
                {all
                  .filter((a) => a.status !== "removed")
                  .map((a) => (
                    <label className="check" key={a.id}>
                      <input
                        type="checkbox"
                        name="member"
                        value={a.id}
                        checked={groupMembers.includes(a.id)}
                        onChange={(e) => {
                          const checked = e.currentTarget.checked;
                          setGroupMembers((previous) =>
                            checked
                              ? [...previous, a.id]
                              : previous.filter((id) => id !== a.id),
                          );
                        }}
                      />
                      <span>
                        {a.name}
                        <small>{statusLabel(a.status)}</small>
                      </span>
                    </label>
                  ))}
              </fieldset>
              <p className="form-help">
                {groupMembers.length
                  ? `${groupMembers.length} ${groupMembers.length === 1 ? "agent selected" : "agents selected"}`
                  : "Select at least one agent to continue."}
              </p>
              <button
                className="primary"
                disabled={busy || !groupMembers.length}
              >
                Create group
              </button>
            </form>
          ) : modal === "policy" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void action(async () => {
                  await api(
                    "/v1/chats/" + encodeURIComponent(selected) + "/policy",
                    policy,
                  );
                  if(room&&policy.mode==='continuous'&&!running)await api('/v1/chats/'+encodeURIComponent(selected)+'/start',{});
                  await refresh();
                  setModal("");
                });
              }}
            >
              <label>
                How should agents work?
                <select
                  value={policy.mode}
                  onChange={(e) =>
                    setPolicy({ ...policy, mode: e.target.value })
                  }
                >
                  <option value="reply">Reply limit</option>
                  <option value="goal">Goal oriented</option>
                  <option value="continuous">Continuous, until paused</option>
                </select>
              </label>
              {policy.mode === "goal" && (
                <label>
                  Goal
                  <textarea
                    required
                    value={policy.goal}
                    onChange={(e) =>
                      setPolicy({ ...policy, goal: e.target.value })
                    }
                  />
                </label>
              )}
              {policy.mode !== "continuous" && (
                <label>
                  Maximum agent replies
                  <input
                    type="number"
                    min="1"
                    max="100"
                    value={policy.limit}
                    onChange={(e) =>
                      setPolicy({ ...policy, limit: Number(e.target.value) })
                    }
                  />
                </label>
              )}
              <p>
                {policy.mode === "continuous"
                  ? "Agents keep taking turns until paused. Each reply uses its host provider."
                  : policy.mode === "goal"
                    ? "Stops on an agent completion report or the reply budget. Review its evidence."
                    : "Agents take turns within this total reply budget."}
              </p>
              <button className="primary" disabled={busy || running}>
                {room&&policy.mode==='continuous'?'Save and start':'Save mode'}
              </button>
            </form>
          ) : modal === "details" ? (
            <>
              <p>{chat?.name}</p>
              <button
                className="control-row"
                onClick={() => setModal("policy")}
              >
                <Settings size={18} />
                Conversation mode
              </button>
              {room ? (
                <>
                  <button
                    className="control-row"
                    onClick={() => setModal("members")}
                  >
                    <Plus size={18} />
                    Add an agent
                  </button>
                  {room.members.map((id) => (
                    <div className="member-row" key={id}>
                      <span>
                        {name(id)}
                        <button className="member-role" aria-label={'Assign role to '+name(id)} onClick={()=>{setRoleMember(id);setRoleDraft(room.roles?.[id]||'');setModal('role')}}>{room.roles?.[id]||'No assigned role'}<Pencil size={14}/></button>
                      </span>
                      <button
                        aria-label={"Stop " + name(id)}
                        onClick={() => void control(id, "stop")}
                      >
                        <Square size={16} />
                        Stop
                      </button>
                      <button
                        aria-label={"Terminate " + name(id)}
                        onClick={() => void control(id, "terminate")}
                      >
                        <Power size={16} />
                        Terminate
                      </button>
                      <button
                        aria-label={"Remove " + name(id) + " from group"}
                        onClick={() =>
                          void action(async () => {
                            await api(
                              "/v1/rooms/" +
                                selected +
                                "/members/" +
                                encodeURIComponent(id) +
                                "/remove",
                              {},
                            );
                            await refresh();
                          })
                        }
                      >
                        <X size={18} />
                        Remove
                      </button>
                    </div>
                  ))}
                  {!room.members.length&&<p>No agents in this group. Add a member to start working together.</p>}
                  <button className="control-row danger" onClick={()=>void action(async()=>{await api('/v1/rooms/'+selected+'/remove',{});await refresh();select('');setModal('');setNotice('Group removed. Its history has been kept.')})}><Trash2 size={18}/>Remove group</button>
                  <p>Removing a member keeps their agent and direct chat. Removing the group stops its work and keeps its history.</p>
                </>
              ) : selected.startsWith("session_") || selected.includes("::") ? (
                <>
                  {chat?.status !== "terminated" &&
                    chat?.status !== "stopped" && (
                      <button
                        className="control-row"
                        onClick={() => void control(selected, "stop")}
                      >
                        <Square size={18} />
                        Stop agent work
                      </button>
                    )}
                  {chat?.status !== "terminated" && (
                    <button
                      className="control-row"
                      onClick={() => void control(selected, "terminate")}
                    >
                      <Power size={18} />
                      Terminate agent
                    </button>
                  )}
                  {(chat?.status === "terminated" ||
                    chat?.status === "stopped") && (
                    <button
                      className="control-row"
                      onClick={() => void control(selected, "resume")}
                    >
                      <Play size={18} />
                      Resume agent
                    </button>
                  )}
                  <button
                    className="control-row danger"
                    onClick={() => void control(selected, "remove")}
                  >
                    <Trash2 size={18} />
                    Remove agent from chats
                  </button>
                  <p>
                    History stays on the host. Removal also removes the agent
                    from groups.
                  </p>
                </>
              ) : (
                <p>Manage remote lifecycle on its host.</p>
              )}
            </>
          ) : modal === "role" ? (
            <form onSubmit={event=>{event.preventDefault();void action(async()=>{await api('/v1/rooms/'+selected+'/members/'+encodeURIComponent(roleMember)+'/role',{role:roleDraft});await refresh();setModal('details')})}}><p>{name(roleMember)} is unassigned unless you give them a role. Coworkers can also find responsibilities naturally while they work.</p><label>Role (optional)<input autoFocus value={roleDraft} maxLength={2000} placeholder="For example, review the mobile layouts" onChange={event=>setRoleDraft(event.currentTarget.value)}/></label><p>Clear the field to leave this coworker unassigned. Changes apply on their next turn.</p><button className="primary" disabled={busy}>Save role</button></form>
          ) : modal === "members" ? (
            <>
              {all
                .filter(
                  (a) =>
                    a.status !== "removed" && !room?.members.includes(a.id),
                )
                .map((a) => (
                  <button
                    className="control-row"
                    key={a.id}
                    onClick={() =>
                      void action(async () => {
                        await api("/v1/rooms/" + selected + "/members", {
                          member: a.id,
                        });
                        await refresh();
                        setModal("details");
                      })
                    }
                  >
                    <Plus size={18} />
                    {a.name}
                  </button>
                ))}
            </>
          ) : modal === "peer" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                const d = new FormData(e.currentTarget);
                void action(async () => {
                  await api("/v1/peers", {
                    name: d.get("name"),
                    url: d.get("url"),
                    token: d.get("token"),
                  });
                  await refresh();
                  setModal("");
                  nav("hosts");
                });
              }}
            >
              <p>
                Use the address and token from <code>mace start</code> on your
                other machine.
              </p>
              <label>
                Name
                <input name="name" placeholder="My laptop" />
              </label>
              <label>
                Host address
                <input
                  name="url"
                  type="url"
                  placeholder="http://192.168.1.20:8080"
                  required
                />
              </label>
              <label>
                Access token
                <input
                  name="token"
                  type="password"
                  autoComplete="off"
                  required
                />
              </label>
              <button className="primary" disabled={busy}>
                Connect host
              </button>
            </form>
          ) : modal === "transfer" ? (
            <>
              <p>Send {preview} to:</p>
              {peers.map((p) => (
                <button
                  key={p.id}
                  className="control-row"
                  disabled={busy}
                  onClick={() =>
                    void action(async () => {
                      await api("/v1/transfers", { peer: p.id, path: preview });
                      setNotice("File transferred and checksum verified.");
                      setModal("");
                    })
                  }
                >
                  <Network size={18} />
                  {p.name}
                </button>
              ))}
            </>
          ) : modal === "logs" ? (
            <Logs id={selected} />
          ) : modal === "more" ? (
            <div className="more-links">
              {[
                ["home", Home, "Home"],
                ["files", Folder, "Files"],
                ["apps", Monitor, "Apps"],
                ["hosts", Network, "Connected hosts"],
                ["manual", BookOpen, "Manual"],
                ["configuration", Terminal, "Configuration"],
              ].map(([id, Icon, label]) => {
                const I = Icon as typeof Home;
                return (
                  <button
                    key={String(id)}
                    onClick={() => {
                      setModal("");
                      nav(String(id));
                      if (id === "files") void browse(".", "page");
                      if (id === "apps") void discoverApps();
                    }}
                  >
                    <I size={20} />
                    {String(label)}
                    <ChevronRight size={17} />
                  </button>
                );
              })}
            </div>
          ) : null}
        </Dialog>
      )}
    </div>
  );
}
function Logs({ id }: { id: string }) {
  const [logs, setLogs] = useState<
    { event_id: string; data: string; type: string }[]
  >([]);
  useEffect(() => {
    let alive = true;
    async function load() {
      try {
        const data = await api<typeof logs>(
          "/v1/agents/" + id + "/events?limit=1000",
        );
        if (alive) setLogs(data);
      } catch {}
    }
    void load();
    const id2 = setInterval(() => void load(), 1500);
    return () => {
      alive = false;
      clearInterval(id2);
    };
  }, [id]);
  return (
    <pre>
      {logs.map((e) => `[${e.type}] ${e.data}`).join("\n") || "No output yet."}
    </pre>
  );
}
function Media({ path }: { path: string }) {
  const url = "/api/v1/files?path=" + encodeURIComponent(path),
    ext = path.split(".").pop()?.toLowerCase() || "";
  return (
    <div className="media">
      {["png", "jpg", "jpeg", "gif", "webp", "avif"].includes(ext) ? (
        <img src={url} alt={path} />
      ) : ["mp4", "webm", "ogv"].includes(ext) ? (
        <video controls preload="metadata" src={url} />
      ) : ["mp3", "wav", "ogg", "m4a", "flac"].includes(ext) ? (
        <audio controls preload="metadata" src={url} />
      ) : [
          "txt",
          "md",
          "json",
          "csv",
          "log",
          "yaml",
          "yml",
          "js",
          "ts",
          "go",
          "py",
          "css",
          "html",
        ].includes(ext) ? (
        <TextPreview url={url} />
      ) : (
        <p>Download to open with a compatible app.</p>
      )}
      <a href={url} download={path.split("/").pop()}>
        <Download size={16} />
        {path.split("/").pop()}
      </a>
    </div>
  );
}
function TextPreview({ url }: { url: string }) {
  const [text, setText] = useState("Loading…");
  useEffect(() => {
    let alive = true;
    void fetch(url, { headers: { Range: "bytes=0-65535" } })
      .then(async (r) => {
        if (!r.ok) throw Error("Cannot read file");
        return r.text();
      })
      .then((t) => {
        if (alive) setText(t.slice(0, 65536));
      })
      .catch((e) => {
        if (alive) setText(String(e));
      });
    return () => {
      alive = false;
    };
  }, [url]);
  return <pre className="text-preview">{text}</pre>;
}
