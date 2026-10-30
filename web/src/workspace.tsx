import { useEffect, useRef, useState, type ReactNode } from "react";
import PublicSite from "./public-site";
import { Button } from "@/components/ui/button";
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
} from "lucide-react";
type Agent = {
  id: string;
  name: string;
  provider?: string;
  status: string;
  working_dir: string;
  labels?: Record<string, string>;
};
type Room = { id: string; name: string; members: string[] };
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
type Run = { id: string; status: string; replies: number; error?: string };
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
const full = (p: string) => (p === "." ? "/" : "/" + p.replace(/^\/+/, ""));
const clock = (s: string) =>
  new Date(s).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
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
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog ref={ref} className="dialog native-dialog" onCancel={close}>
      <header>
        <h2>{title}</h2>
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
    [checking, setChecking] = useState(true),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [notice, setNotice] = useState("");
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
    [drafts, setDrafts] = useState<Record<string, string>>({}),
    [search, setSearch] = useState(""),
    [filter, setFilter] = useState("all"),
    [modal, setModal] = useState(""),
    [attachment, setAttachment] = useState(""),
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
  const bottom = useRef<HTMLDivElement>(null);
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
    id === "you" ? "You" : all.find((a) => a.id === id)?.name || id;
  const nav = (p: string) => {
    location.hash = p;
    setPage(p);
  };
  async function action(fn: () => Promise<void>) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function refresh() {
    try {
      const session = await api<{ connected: boolean }>("/session");
      if (!session.connected) {
        setConnected(false);
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
    } catch {
      setConnected(false);
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
      } catch (e) {
        if (alive) setError((e as Error).message);
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
    bottom.current?.scrollIntoView({ block: "end" });
  }, [messages.length, selected]);
  useEffect(() => {
    if (!connected || !["activity", "host"].includes(page)) return;
    let alive = true;
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
      }
    }
    void load();
    const id = setInterval(() => void load(), 3000);
    return () => {
      alive = false;
      clearInterval(id);
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
      await api("/v1/sessions/" + id + "/control", { action: operation });
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
            <Folder size={20} />
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
        {!connected ? (
          <div className="connect-screen">
            <img src="/mace-mark.svg" width="64" height="64" alt="Mace" />
            <h1>{checking ? "Connecting…" : "Your host is one click away."}</h1>
            <p>
              Open the sign-in link printed by <code>mace start</code>.
            </p>
            {!checking && (
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
                        setAttachment("");
                      }}
                    >
                      <div className="avatar">
                        {c.status === "group" ? <Users /> : <Terminal />}
                      </div>
                      <div className="chatcopy">
                        <div>
                          <strong>{c.name}</strong>
                          <small>{c.status}</small>
                        </div>
                        <p>
                          {c.status === "group"
                            ? `${rooms.find((r) => r.id === c.id)?.members.length} agents`
                            : c.working_dir}
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
                <span className="online" />
                <div>
                  <strong>
                    {String(node.host_name || node.name || "Your host")}
                  </strong>
                  <small>Connected · private workspace</small>
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
                      <small>
                        {running
                          ? "Working · " + run.replies + " replies"
                          : chat.status + " · " + chat.working_dir}
                      </small>
                    </div>
                    <button
                      aria-label="Pause conversation"
                      disabled={!running || busy}
                      onClick={() =>
                        void action(async () => {
                          await api(
                            "/v1/chats/" +
                              encodeURIComponent(selected) +
                              "/pause",
                            {},
                          );
                          await refresh();
                        })
                      }
                    >
                      <Pause size={20} />
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
                          : "Reply limit · " + policy.limit + " replies"}
                    </span>
                    <span>
                      Change <ChevronRight size={14} />
                    </span>
                  </button>
                  <div className="messages">
                    <div className="privacy-note">
                      Files stay on your host unless you share them with another computer.
                    </div>
                    {!messages.length && (
                      <div className="startnote">
                        What would you like {chat.name} to do?
                      </div>
                    )}
                    {messages.map((m) => (
                      <article
                        key={m.id}
                        className={
                          "bubble " +
                          (m.sender === "you" ? "outgoing" : "incoming")
                        }
                      >
                        <strong className="sender">{name(m.sender)}</strong>
                        <div className="message-text">{m.text}</div>
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
                    {run?.error && (
                      <div className="run-error" role="alert">
                        {run.error}
                        <button onClick={() => nav("activity")}>
                          View activity
                        </button>
                      </div>
                    )}
                    {run && !running && (
                      <div className="work-state">
                        {run.status.replaceAll("_", " ")} · {run.replies}{" "}
                        replies
                      </div>
                    )}
                    <div ref={bottom} />
                  </div>
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
                      if (running || busy) return;
                      void action(async () => {
                        const m = await api<Msg>(
                          "/v1/chats/" +
                            encodeURIComponent(selected) +
                            "/messages",
                          { text: drafts[selected] || "", file: attachment },
                        );
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
                      aria-label="Message"
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
                        if (e.key === "Enter" && !e.shiftKey) {
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
                        running ||
                        (!(drafts[selected] || "").trim() && !attachment)
                      }
                    >
                      <Send size={22} />
                    </button>
                  </form>
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
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                  />
                </label>
                <div className="apps-list">
                  {apps
                    .filter((a) =>
                      a.name.toLowerCase().includes(search.toLowerCase()),
                    )
                    .map((a) => (
                      <button
                        key={a.id}
                        disabled={busy}
                        onClick={() => void open(a.id)}
                      >
                        <Monitor size={20} />
                        {a.name}
                        <ChevronRight size={17} />
                      </button>
                    ))}
                </div>
                {!apps.length && (
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
                <h2>Conversations</h2>
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
                      {r.status.replaceAll("_", " ")} · {r.replies} replies
                    </small>
                  </button>
                ))}
                <h2>Event log</h2>
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
                <h2>File transactions</h2>
                {transfers.map((t) => (
                  <details key={t.id}>
                    <summary>
                      {t.source} · {t.bytes} bytes · {t.status}
                    </summary>
                    <pre>{JSON.stringify(t, null, 2)}</pre>
                  </details>
                ))}
                <h2>Native process logs</h2>
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
      {error && (
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
            }[modal] || modal
          }
          close={() => setModal("")}
        >
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
                void action(async () => {
                  const r = await api<Room>("/v1/rooms", {
                    name: d.get("name"),
                    members: d.getAll("member"),
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
                      <input type="checkbox" name="member" value={a.id} />
                      {a.name}
                      <small>{a.status}</small>
                    </label>
                  ))}
              </fieldset>
              <button className="primary" disabled={busy || !all.length}>
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
                Save mode
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
                      <span>{name(id)}</span>
                      <button
                        aria-label={"Stop " + name(id)}
                        onClick={() => void control(id, "stop")}
                      >
                        <Square size={16} />
                      </button>
                      <button
                        aria-label={"Terminate " + name(id)}
                        onClick={() => void control(id, "terminate")}
                      >
                        <Power size={16} />
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
                      </button>
                    </div>
                  ))}
                </>
              ) : selected.startsWith("session_") || selected.includes("::") ? (
                <>
                  <button
                    className="control-row"
                    onClick={() => void control(selected, "stop")}
                  >
                    <Square size={18} />
                    Stop agent work
                  </button>
                  <button
                    className="control-row"
                    onClick={() => void control(selected, "terminate")}
                  >
                    <Power size={18} />
                    Terminate agent
                  </button>
                  <button
                    className="control-row"
                    onClick={() => void control(selected, "resume")}
                  >
                    <Play size={18} />
                    Resume agent
                  </button>
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
