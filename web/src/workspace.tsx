import {
  Fragment,
  lazy,
  Suspense,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import PublicSite from "./public-site";
import { Machines, machineBase, type Machine } from "./machines";
import { Button } from "@/components/ui/button";
const AgentMessage = lazy(() => import("./agent-message"));
import { StepList, WorkCard, type Step } from "./agent-work";
import { MaceLogo, MaceMark, WorkingPills } from "./brand";
import { ActivityBars, EventDetails, EventIcon, actorLabel, eventTitle, type Sample } from "./charts";

import {
  AppTile,
  Avatar,
  PlainWithMentions,
  parseCommand,
  useCommandMenu,
  type CommandOption,
  dayLabel,
  listTime,
  plainPreview,
  colorProfiles,
  profileFor,
  useMentionMenu,
  type Suggestion,
} from "./messenger-parts";
import {
  applyTheme,
  palettes,
  resolve,
  setWallpaper,
  themePref,
  wallpaperPref,
  type ThemePref,
} from "./theme";
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
  Check,
  CheckCheck,
  AtSign,
  Slash,
  FolderOpen,
  Cpu,
} from "lucide-react";
type Agent = {
  id: string;
  name: string;
  handle?: string;
  model?: string;
  role?: string;
  color?: number;
  avatar?: boolean;
  provider?: string;
  status: string;
  working_dir: string;
  labels?: Record<string, string>;
};
type Room = { id: string; name: string; members: string[]; roles?:Record<string,string>; claimed?: Record<string, string>; flairs?: string[]; parent?: string };
type Msg = {
  id: string;
  room?: string;
  seq?: number;
  sender: string;
  kind?: string;
  text: string;
  file?: string;
  at: string;
  delivery?: Record<string, string>;
  mentions?: string[];
  activity?: Step[];
  flair?: string;
  role?: string;
  origin?: string;
};
type MachineNode = { id: string; name: string; self?: boolean; available: boolean; outdated?: boolean };
type Entry = { name: string; path: string; directory: boolean; size: number };
type Provider = { id: string; name: string; available: boolean };
type Peer = { id: string; name: string; url: string };
type Policy = { mode: string; goal: string; limit: number; tone?: string; lead?: string };
type Run = { id: string; status: string; replies: number; error?: string; active_member?:string; active_members?: string[] };
// Messages arrive from the stream and from paged loads; keep one copy of each,
// in conversation order.
const mergeMessages = (list: Msg[], incoming: Msg[]) => {
  const byID = new Map(list.map((m) => [m.id, m]));
  for (const m of incoming) byID.set(m.id, { ...byID.get(m.id), ...m });
  // Time orders the chat, since a chat can merge messages written on another
  // machine, which carry no position here; position breaks ties.
  return [...byID.values()].sort(
    (a, b) =>
      Date.parse(a.at) - Date.parse(b.at) || (a.seq ?? Infinity) - (b.seq ?? Infinity),
  );
};
const joinNames = (names: string[]) =>
  names.length < 3
    ? names.join(" and ")
    : names.slice(0, -1).join(", ") + ", and " + names.at(-1);
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
// Push needs a service worker, which needs HTTPS or localhost.
const pushSupported = () =>
  window.isSecureContext && "serviceWorker" in navigator && "PushManager" in window;
const base64URLBytes = (s: string) => {
  const raw = atob((s + "=".repeat((4 - (s.length % 4)) % 4)).replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(raw, (c) => c.charCodeAt(0));
};
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
const readableError=(error:unknown)=>{const message=(error as Error).message||String(error);if(/dial tcp|connection refused|no such host|i\/o timeout/.test(message))return 'Could not reach that host. Check its address and make sure MACE is running there.';if(/Failed to fetch|NetworkError|Load failed/.test(message))return 'The host is unavailable. Your draft is kept. Check the connection and try again.';if(/^unauthorized$/i.test(message.trim()))return 'That access token was not accepted. Use the token printed by the running host.';return message};
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
      initial.startsWith("connect") ? "messenger" : initial || "home",
    ),
    [pref, setPref] = useState<ThemePref>(themePref),
    [wallpaper, setWallpaperOn] = useState(wallpaperPref),
    [keepWorking, setKeepWorking] = useState(false),
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
    [machines, setMachines] = useState<MachineNode[]>([]),
    [phoneLink, setPhoneLink] = useState<{ link: string; svg: string } | null>(null),
    [pushOn, setPushOn] = useState(false),
    [remote, setRemote] = useState<Agent[]>([]),
    [runs, setRuns] = useState<Run[]>([]),
    [node, setNode] = useState<Record<string, unknown>>({});
  const [selected, select] = useState(""),
    [messages, setMessages] = useState<Msg[]>([]),
    [olderAvailable, setOlderAvailable] = useState(false),
    [streamOpen, setStreamOpen] = useState(false),
    [reloadKey, setReloadKey] = useState(0),
    // Live tool steps of agents working right now, by conversation and agent.
    [live, setLive] = useState<Record<string, Record<string, Step[]>>>({}),
    [drafts, setDrafts] = useState<Record<string, string>>(() => {
      try {
        return JSON.parse(sessionStorage.getItem("mace-drafts") || "{}");
      } catch {
        return {};
      }
    }),
    [searches, setSearches] = useState<Record<string, string>>({}),
    [filter, setFilter] = useState("all"),
    [newMenu, setNewMenu] = useState(false),
    [inbox, setInbox] = useState<Record<string, { latest: Msg; unread: number }>>({}),
    [receipts, setReceipts] = useState<Record<string, number>>({}),
    [caret, setCaret] = useState(0),
    [machine, setMachine] = useState<{ host: string; history: Sample[] }>({ host: "", history: [] }),
    [modal, setModal] = useState(""),
    [attachments, setAttachments] = useState<Record<string, string>>({}),
    [groupMembers, setGroupMembers] = useState<string[]>([]),
    [unread, setUnread] = useState(false),
    [roleMember,setRoleMember]=useState(''),[roleDraft,setRoleDraft]=useState(''),
    [profileDraft, setProfileDraft] = useState({ name: "", handle: "", role: "", color: 0 }),
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
    // The machine a previewed file lives on; null is this computer.
    [previewOn, setPreviewOn] = useState<Machine | null>(null),
    [activity, setActivity] = useState<Audit[]>([]),
    [transfers, setTransfers] = useState<Transfer[]>([]),
    [apps, setApps] = useState<{ id: string; name: string; category?: string }[]>([]);
  const [activityLoading, setActivityLoading] = useState(true),
    [appsLoading, setAppsLoading] = useState(false);
  const bottom = useRef<HTMLDivElement>(null);
  const messagePane = useRef<HTMLDivElement>(null),
    composer = useRef<HTMLTextAreaElement>(null),
    nearBottom = useRef(true),
    busyNow = useRef(false),
    selectedRef = useRef(""),
    streamOpenRef = useRef(false),
    refreshRef = useRef<() => Promise<void>>(async () => {});
  selectedRef.current = selected;
  streamOpenRef.current = streamOpen;
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
    running = run?.status === "running",
    liveHere = live[selected] || {},
    working = [
      ...new Set([...(run?.active_members || []), ...Object.keys(liveHere)]),
    ];
  const dark = palettes.find((p) => p.id === resolve(pref))?.scheme === "dark";
  // The quick toggle moves between the two everyday palettes.
  const toggleScheme = () => setPref(dark ? "signal" : "ink");
  const handles = (room ? room.members : selected ? [selected] : [])
    .map((id) => all.find((a) => a.id === id)?.handle)
    .filter((h): h is string => !!h);
  const people: Suggestion[] = (room ? room.members : [])
    .map((id) => all.find((a) => a.id === id))
    .filter((a): a is Agent => !!a?.handle)
    .map((a) => ({ id: a.id, handle: a.handle!, name: a.name, detail: statusLabel(a.status), provider: a.provider }));
  if (room)
    people.push({ id: "all", handle: "all", name: "Everyone", detail: "Every agent in this group" });
  const draft = drafts[selected] || "";
  const mention = useMentionMenu(draft, caret, people);
  const pickMention = (p: Suggestion) => {
    const { next, caret: at } = mention.insert(p);
    setDrafts((v) => ({ ...v, [selected]: next }));
    requestAnimationFrame(() => {
      composer.current?.focus();
      composer.current?.setSelectionRange(at, at);
      setCaret(at);
    });
  };
  // Commands work in private chats only; groups keep / as plain text.
  const commandAgent = !room && selected ? all.find((a) => a.id === selected) : undefined;
  const commands = useCommandMenu(
    draft,
    caret,
    !!commandAgent,
    commandAgent ? { id: commandAgent.id, folder: commandAgent.working_dir } : null,
    api,
  );
  const runCommand = (text: string, confirmed = false) => {
    const c = parseCommand(text);
    if (!c || !commandAgent) return;
    const agent = commandAgent;
    if (c.name === "files") {
      setDrafts((v) => ({ ...v, [agent.id]: "" }));
      nav("files");
      void browse(agent.working_dir, "page");
      return;
    }
    if (c.name === "new" && !confirmed) {
      setModal("new-session");
      return;
    }
    void action(async () => {
      const r = await api<{ note: Msg; session: Agent }>(
        "/v1/sessions/" + encodeURIComponent(agent.id) + "/command",
        c,
      );
      nearBottom.current = true;
      setMessages((v) => (selectedRef.current === agent.id ? mergeMessages(v, [r.note]) : v));
      setAgents((v) => v.map((a) => (a.id === agent.id ? { ...a, ...r.session, id: a.id } : a)));
      setDrafts((v) => ({ ...v, [agent.id]: "" }));
    });
  };
  const pickCommand = (o: CommandOption) => {
    setDrafts((v) => ({ ...v, [selected]: o.text }));
    if (o.run) return runCommand(o.text);
    requestAnimationFrame(() => {
      composer.current?.focus();
      composer.current?.setSelectionRange(o.text.length, o.text.length);
      setCaret(o.text.length);
    });
  };
  // Group members keep one name tone, so speakers are easy to tell apart.
  const senderTone = (id: string) => ((room?.members.indexOf(id) ?? 0) % 4) + 1;
  // Everything an avatar needs to look like this particular agent.
  const look = (id: string) => {
    const a = all.find((x) => x.id === id);
    return { id, provider: a?.provider, color: a?.color, handle: a?.handle, ownerLogo: !!a?.avatar };
  };
  const openFile = (path: string) => {
    setPreview(path);
    setModal("preview");
  };
  const name = (id: string) =>
    id === "you" ? "You" : id==='mace'?'MACE':all.find((a) => a.id === id)?.name || id;
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
  refreshRef.current = refresh;
  useEffect(() => {
    let alive = true;
    async function init() {
      try {
        if (initial.startsWith("connect=")) {
          await api("/session", {
            token: decodeURIComponent(initial.slice(8)),
          });
          history.replaceState(null, "", "#messenger");
        } else if (initial.startsWith("connect-once=")) {
          // A QR code or another of the owner's machines: a one-time code.
          await api("/session", { code: decodeURIComponent(initial.slice(13)) });
          history.replaceState(null, "", "#messenger");
        }
        const room = new URLSearchParams(location.search).get("room");
        if (room) {
          select(room);
          history.replaceState(null, "", "/#messenger");
        }
        if (alive) await refresh();
      } catch (e) {
        setError((e as Error).message);
      } finally {
        if (alive) setChecking(false);
      }
    }
    void init();
    // With the event stream open, a full refresh is only a slow safety net.
    let last = Date.now();
    const id = setInterval(() => {
      if (streamOpenRef.current && Date.now() - last < 30000) return;
      last = Date.now();
      void refreshRef.current();
    }, 3000);
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
    if (connected && page === "settings") {
      void api<Device[]>("/v1/devices")
        .then(setDevices)
        .catch(() => setDevices([]));
      void api<{ keep_working: boolean }>("/v1/settings/presence")
        .then((r) => setKeepWorking(r.keep_working))
        .catch(() => {});
    }
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
    applyTheme(pref);
    if (pref !== "auto") return;
    // Auto follows the system as it changes, such as at sunset.
    const media = matchMedia("(prefers-color-scheme: dark)");
    const follow = () => applyTheme("auto");
    media.addEventListener("change", follow);
    return () => media.removeEventListener("change", follow);
  }, [pref]);
  useEffect(() => setWallpaper(wallpaper), [wallpaper]);
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
  // A tapped notification opens its conversation in this window.
  useEffect(() => {
    if (!("serviceWorker" in navigator)) return;
    const open = (e: MessageEvent) => {
      if (e.data?.type === "open-room" && e.data.room) {
        select(e.data.room);
        nav("messenger");
      }
    };
    navigator.serviceWorker.addEventListener("message", open);
    return () => navigator.serviceWorker.removeEventListener("message", open);
  }, []);
  useEffect(() => {
    if (page !== "settings" || !pushSupported()) return;
    void navigator.serviceWorker.ready
      .then((r) => r.pushManager.getSubscription())
      .then((sub) => setPushOn(!!sub))
      .catch(() => {});
  }, [page]);
  async function togglePush() {
    const reg = await navigator.serviceWorker.ready;
    const current = await reg.pushManager.getSubscription();
    if (current) {
      await api("/v1/push/unsubscribe", current.toJSON());
      await current.unsubscribe();
      setPushOn(false);
      return;
    }
    if ((await Notification.requestPermission()) !== "granted")
      throw Error("Notifications are blocked for this site. Allow them in the browser's site settings.");
    const { key } = await api<{ key: string }>("/v1/push/key");
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: base64URLBytes(key),
    });
    await api("/v1/push/subscribe", sub.toJSON());
    setPushOn(true);
  }
  // Joined machines and whether each is reachable, for offline notices.
  useEffect(() => {
    if (!connected) return;
    const load = () =>
      void api<MachineNode[]>("/v1/nodes")
        .then(setMachines)
        .catch(() => setMachines([]));
    load();
    const id = setInterval(load, 15000);
    return () => clearInterval(id);
  }, [connected]);
  // Load the newest page; the event stream keeps it current. Polling only
  // runs while the stream is down.
  useEffect(() => {
    if (!selected) return;
    let alive = true;
    async function load() {
      try {
        const r = await fetch(
          "/api/v1/chats/" + encodeURIComponent(selected) + "/messages?limit=100",
        );
        if (!r.ok) throw Error((await r.text()).trim());
        const page = (await r.json()) as Msg[];
        if (!alive) return;
        setMessages((v) =>
          mergeMessages(
            v.filter((m) => m.room === selected || !m.room),
            page,
          ),
        );
        setOlderAvailable((more) => more || r.headers.get("X-More") === "true");
      } catch {
        if (alive) setOnline(false);
      }
    }
    void load();
    if (streamOpen) return () => void (alive = false);
    const id = setInterval(() => void load(), 2000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [selected, streamOpen, reloadKey]);
  useEffect(() => {
    setMessages([]);
    setOlderAvailable(false);
  }, [selected]);
  // The chat list: newest message and unread count per conversation. What has
  // been seen is remembered per browser.
  useEffect(() => {
    if (!connected) return;
    let seen: Record<string, number> = {};
    try {
      seen = JSON.parse(stored("mace-seen") || "{}");
    } catch {
      /* A damaged record only resets unread counts. */
    }
    void api<{ room: string; latest: Msg; unread: number }[]>("/v1/inbox", { seen })
      .then((rows) =>
        setInbox(Object.fromEntries(rows.map((r) => [r.room, { latest: r.latest, unread: r.unread }]))),
      )
      .catch(() => {});
  }, [connected, reloadKey]);
  useEffect(() => {
    if (!selected) return;
    const last = messages.reduce((n, m) => Math.max(n, m.seq || 0), 0);
    if (!last) return;
    try {
      const seen = JSON.parse(stored("mace-seen") || "{}");
      if ((seen[selected] || 0) < last) store("mace-seen", JSON.stringify({ ...seen, [selected]: last }));
    } catch {
      store("mace-seen", JSON.stringify({ [selected]: last }));
    }
    setInbox((v) => (v[selected]?.unread ? { ...v, [selected]: { ...v[selected], unread: 0 } } : v));
  }, [selected, messages]);
  useEffect(() => {
    setReceipts({});
    if (!selected || !connected) return;
    void api<Record<string, number>>("/v1/chats/" + encodeURIComponent(selected) + "/receipts")
      .then(setReceipts)
      .catch(() => {});
  }, [selected, connected]);
  useEffect(() => {
    if (!connected || page !== "host") return;
    void api<{ host: string; history: Sample[] | null }>("/v1/metrics")
      .then((m) => setMachine({ host: m.host, history: m.history || [] }))
      .catch(() => {});
  }, [connected, page]);
  async function loadOlder() {
    const first = messages.find((m) => m.seq);
    const pane = messagePane.current;
    if (!first?.seq || !pane) return;
    const before = pane.scrollHeight;
    const r = await fetch(
      "/api/v1/chats/" +
        encodeURIComponent(selected) +
        "/messages?limit=100&before=" +
        first.seq,
    );
    if (!r.ok) throw Error((await r.text()).trim());
    const page = (await r.json()) as Msg[];
    setOlderAvailable(r.headers.get("X-More") === "true");
    setMessages((v) => mergeMessages(v, page));
    // Keep the reader's place while earlier messages appear above.
    requestAnimationFrame(() => {
      pane.scrollTop += pane.scrollHeight - before;
    });
  }
  // One event stream carries messages, runs, sessions, and live agent steps.
  // EventSource reconnects by itself and resumes from the last event it saw.
  useEffect(() => {
    if (!connected) return;
    const stream = new EventSource("/api/v1/stream");
    const data = <T,>(e: Event) => JSON.parse((e as MessageEvent).data) as T;
    stream.onopen = () => setStreamOpen(true);
    stream.onerror = () => setStreamOpen(false);
    stream.addEventListener("reset", () => {
      setLive({});
      setReloadKey((k) => k + 1);
      void refreshRef.current();
    });
    stream.addEventListener("message", (e) => {
      const m = data<Msg>(e);
      if (m.room === selectedRef.current)
        setMessages((v) => mergeMessages(v, [m]));
      if (m.room && m.seq)
        setInbox((v) => {
          const prev = v[m.room!];
          if (prev && (prev.latest.seq ?? 0) > m.seq!) return v;
          const fresh = !prev || prev.latest.id !== m.id;
          const counts =
            fresh && m.room !== selectedRef.current && m.sender !== "you";
          return {
            ...v,
            [m.room!]: { latest: m, unread: (prev?.unread || 0) + (counts ? 1 : 0) },
          };
        });
    });
    // Records synced from another machine can belong to the open chat.
    stream.addEventListener("records", () => setReloadKey((k) => k + 1));
    stream.addEventListener("metrics", (e) => {
      const m = data<{ host: string; sample: Sample }>(e);
      if (m.host === "local")
        setMachine((v) => ({ ...v, history: [...v.history, m.sample].slice(-120) }));
    });
    stream.addEventListener("cursor", (e) => {
      const c = data<{ room: string; member: string; seq: number }>(e);
      if (c.room === selectedRef.current)
        setReceipts((v) => ({ ...v, [c.member]: Math.max(v[c.member] || 0, c.seq) }));
    });
    stream.addEventListener("run", (e) => {
      const r = data<Run>(e);
      setRuns((v) => [...v.filter((x) => x.id !== r.id), r]);
    });
    stream.addEventListener("session", (e) => {
      const s = data<Agent>(e);
      setAgents((v) => v.map((a) => (a.id === s.id ? { ...a, ...s } : a)));
    });
    stream.addEventListener("activity", (e) => {
      const a = data<{ room: string; member: string; steps?: Step[]; done?: boolean }>(e);
      setLive((v) => {
        const roomSteps = { ...(v[a.room] || {}) };
        if (a.done) delete roomSteps[a.member];
        else roomSteps[a.member] = a.steps || [];
        return { ...v, [a.room]: roomSteps };
      });
    });
    return () => {
      stream.close();
      setStreamOpen(false);
    };
  }, [connected]);
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
  // One agent with its controls, on whichever machine it runs.
  const agentRow = (a: Agent) => (
    <div className="agent-row" key={a.id}>
      <button
        className="agent-open"
        onClick={() => {
          select(a.id);
          nav("messenger");
        }}
      >
        <Avatar {...look(a.id)} size={38} working={a.status === "replying"} />
        <span>
          <strong>{a.name}</strong>
          <small title={a.working_dir}>
            @{a.handle} · {folderName(a.working_dir)}
          </small>
        </span>
        <em className={"status-chip " + a.status}>{statusLabel(a.status)}</em>
      </button>
      <div className="agent-actions">
        <button className="icon-button" title="Stop" aria-label={"Stop " + a.name} onClick={() => void control(a.id, "stop")}>
          <Square size={16} />
        </button>
        <button className="icon-button" title="Terminate" aria-label={"Terminate " + a.name} onClick={() => void control(a.id, "terminate")}>
          <Power size={16} />
        </button>
        <button className="icon-button danger" title="Remove" aria-label={"Remove " + a.name} onClick={() => void control(a.id, "remove")}>
          <Trash2 size={16} />
        </button>
      </div>
    </div>
  );
  // Starts an agent on this computer or on a joined machine, then opens it.
  async function startOn(m: Machine, provider: string, folder: string) {
    const a = m.self
      ? await api<Agent>("/v1/sessions", { provider, working_dir: folder })
      : await api<Agent>("/v1/peers/" + encodeURIComponent(m.id) + "/sessions", { provider, working_dir: folder });
    if (!m.self) setRemote((v) => [...v.filter((x) => x.id !== a.id), a]);
    await refresh();
    select(a.id);
    nav("messenger");
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
      {purpose === "attach" && path !== "." && (
        <button
          className="secondary"
          onClick={() => {
            setAttachment(path.replace(/\/+$/, "") + "/");
            setModal("");
          }}
        >
          Attach this whole folder
        </button>
      )}
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
        toggleTheme={toggleScheme}
        connected={connected}
      />
    );
  return (
    <div className="app">
      <nav className="rail" aria-label="Main navigation">
        <a className="brand" href="#home" aria-label="MACE home">
          <MaceMark title="MACE home" inverse />
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
          aria-label={dark ? "Use a light theme" : "Use a dark theme"}
          onClick={toggleScheme}
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
            <MaceLogo assemble className="connect-logo" />
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
                <h1>Chats</h1>
                <div className="menu-anchor">
                  <button
                    className="icon-button accent"
                    aria-label="New chat"
                    aria-haspopup="menu"
                    aria-expanded={newMenu}
                    onClick={() => setNewMenu(!newMenu)}
                  >
                    <Plus size={22} />
                  </button>
                  {newMenu && (
                    <div className="popover-menu" role="menu" onKeyDown={(e) => e.key === "Escape" && setNewMenu(false)}>
                      <button role="menuitem" autoFocus onClick={() => { setNewMenu(false); setModal("launch"); }}>
                        <Terminal size={18} />
                        <span>
                          New agent
                          <small>Start an installed CLI agent</small>
                        </span>
                      </button>
                      <button role="menuitem" onClick={() => { setNewMenu(false); setModal("group"); }}>
                        <Users size={18} />
                        <span>
                          New group
                          <small>Agents that work together</small>
                        </span>
                      </button>
                    </div>
                  )}
                </div>
              </header>
              <label className="search">
                <Search size={17} />
                <input
                  aria-label="Search chats"
                  placeholder="Search"
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </label>
              {chats.length > 6 && rooms.length > 0 && (
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
              )}
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
                  .sort(
                    (a, b) =>
                      (inbox[b.id]?.latest.seq ?? 0) - (inbox[a.id]?.latest.seq ?? 0),
                  )
                  .map((c) => {
                    const group = c.status === "group";
                    const entry = inbox[c.id];
                    const busyHere = Object.entries(live[c.id] || {});
                    const step = busyHere
                      .flatMap(([who, steps]) => steps.map((s) => ({ who, s })))
                      .filter((x) => x.s.state === "running")
                      .at(-1);
                    const unread = selected === c.id ? 0 : entry?.unread || 0;
                    const preview = entry?.latest;
                    return (
                      <button
                        className={
                          "conversation" +
                          (selected === c.id ? " selected" : "") +
                          (unread ? " unread" : "")
                        }
                        key={c.id}
                        onClick={() => select(c.id)}
                      >
                        <Avatar {...look(c.id)} group={group} working={busyHere.length > 0} />
                        <div className="chatcopy">
                          <div>
                            <strong>
                              {c.name}
                              {group && rooms.find((r) => r.id === c.id)?.parent && (
                                <em className="side-label"> · side chat in {rooms.find((r) => r.id === rooms.find((x) => x.id === c.id)?.parent)?.name}</em>
                              )}
                            </strong>
                            <small>{preview ? listTime(preview.at) : ""}</small>
                          </div>
                          <div>
                            {busyHere.length ? (
                              <p className="list-working">
                                <WorkingPills />
                                <span>
                                  {step
                                    ? (group ? name(step.who) + " · " : "") +
                                      step.s.title.replaceAll("`", "")
                                    : joinNames(busyHere.map(([who]) => name(who))) +
                                      (busyHere.length === 1 ? " is working" : " are working")}
                                </span>
                              </p>
                            ) : (
                              <p>
                                {preview
                                  ? (preview.sender === "you"
                                      ? "You: "
                                      : group && preview.sender !== "mace"
                                        ? name(preview.sender) + ": "
                                        : "") + plainPreview(preview.text)
                                  : group
                                    ? `${rooms.find((r) => r.id === c.id)?.members.length} agents`
                                    : "Say hello to start"}
                              </p>
                            )}
                            {unread > 0 && (
                              <span className="badge" aria-label={unread + " unread"}>
                                {unread > 99 ? "99+" : unread}
                              </span>
                            )}
                          </div>
                        </div>
                      </button>
                    );
                  })}
                {!chats.length && (
                  <div className="emptylist">
                    <MaceMark className="empty-mark" title="" />
                    <h2>Who shall we work with?</h2>
                    <p>Pick an agent installed on this computer.</p>
                    <button
                      className="primary"
                      onClick={() => setModal("launch")}
                    >
                      Choose an agent <Plus size={17} />
                    </button>
                  </div>
                )}
              </div>
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
                    <Avatar {...look(chat.id)} group={!!room} size={40} working={working.length > 0} />
                    <div className="threadtitle">
                      <strong>{chat.name}</strong>
                      <small title={chat.working_dir}>
                        {working.length
                          ? joinNames(working.map(name)) +
                            (working.length === 1 ? " is working" : " are working")
                          : running
                            ? "Waiting for the next turn"
                            : room
                              ? room.members.map((id) => "@" + (all.find((a) => a.id === id)?.handle || name(id))).join(", ")
                              : statusLabel(chat.status) +
                            (chat.working_dir
                              ? " · " + folderName(chat.working_dir)
                              : "")}
                      </small>
                    </div>
                    {running && (
                      <button
                        className="icon-button pause-now"
                        aria-label="Pause this conversation"
                        title="Pause"
                        disabled={busy}
                        onClick={() =>
                          void action(async () => {
                            await api("/v1/chats/" + encodeURIComponent(selected) + "/pause", {});
                            await refresh();
                          })
                        }
                      >
                        <Pause size={20} />
                      </button>
                    )}
                    <button
                      aria-label="Conversation controls"
                      onClick={() => setModal("details")}
                    >
                      <MoreVertical />
                    </button>
                  </header>
                  {(() => {
                    const away = machines.filter(
                      (n) =>
                        !n.self &&
                        !n.available &&
                        (room ? room.members : [selected]).some((id) => id.startsWith(n.id + "::")),
                    );
                    return away.length ? (
                      <div className="connection-banner offline-machine" role="status">
                        <WifiOff size={18} />
                        <span>
                          {joinNames(away.map((n) => n.name))}{" "}
                          {away.length === 1 ? "is" : "are"} offline. Agents there answer once{" "}
                          {away.length === 1 ? "it is" : "they are"} back; messages written there show up then.
                        </span>
                      </div>
                    ) : null;
                  })()}
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
                    {olderAvailable && (
                      <button
                        className="load-older"
                        disabled={busy}
                        onClick={() => void action(loadOlder)}
                      >
                        Load earlier messages
                      </button>
                    )}
                    {!messages.length && (
                      <div className="startnote">
                        What would you like {chat.name} to do?
                      </div>
                    )}
                    {messages.map((m, i) => {
                      const prev = messages[i - 1];
                      const newDay = !prev || dayLabel(prev.at) !== dayLabel(m.at);
                      const grouped =
                        !!prev &&
                        !newDay &&
                        prev.sender === m.sender &&
                        m.sender !== "mace" &&
                        new Date(m.at).getTime() - new Date(prev.at).getTime() < 5 * 60000;
                      const mine = m.sender === "you";
                      const addressed = m.mentions?.length ? m.mentions : room ? room.members : [selected];
                      const read =
                        mine && !!m.seq && addressed.length > 0 &&
                        addressed.every((id) => (receipts[id] || 0) >= m.seq!);
                      const failed = Object.values(m.delivery || {}).some((v) => v !== "delivered");
                      return (
                        <Fragment key={m.id}>
                          {newDay && (
                            <div className="day-divider" role="separator">
                              <span>{dayLabel(m.at)}</span>
                            </div>
                          )}
                          <article
                            className={
                              (m.kind === "note" ? "command-note " : m.sender === "mace" ? "system-message " : "bubble ") +
                              (mine ? "outgoing" : "incoming") +
                              (grouped ? " grouped" : "")
                            }
                          >
                            {!mine && m.sender !== "mace" && room && !grouped && (
                              <strong className={"sender who-" + (senderTone(m.sender))}>
                                {name(m.sender)}
                              </strong>
                            )}
                            {(m.flair || m.role) && (
                              <div className="msg-tags">
                                {m.flair && <span className={"flair-chip flair-" + m.flair.replaceAll(" ", "-")}>{m.flair}</span>}
                                {m.role && <span className="role-chip">took on: {m.role}</span>}
                              </div>
                            )}
                            <div className={"message-text " + (mine || m.kind === "note" ? "plain" : "markdown")}>
                              {m.kind === "note" ? (
                                m.text
                              ) : mine ? (
                                <PlainWithMentions text={m.text} handles={handles} />
                              ) : (
                                <Suspense fallback={<span>{m.text}</span>}>
                                  <AgentMessage text={m.text} handles={handles} onFile={openFile} />
                                </Suspense>
                              )}
                            </div>
                            {m.activity && <StepList steps={m.activity} />}
                            {m.file && (
                              <Media
                                path={m.file}
                                onFolder={(p) => {
                                  nav("files");
                                  void browse(p, "page");
                                }}
                              />
                            )}
                            {m.sender !== "mace" && (
                              <small className="meta">
                                {failed && <span className="meta-failed">Not delivered · </span>}
                                {m.origin && <span className="meta-origin">on {m.origin} · </span>}
                                {clock(m.at)}
                                {mine && !failed && (
                                  <span
                                    className={"receipt" + (read ? " read" : "")}
                                    aria-label={read ? "Read by the agents" : "Sent"}
                                    title={read ? "Read by the agents" : "Sent"}
                                  >
                                    {read ? <CheckCheck size={15} /> : <Check size={15} />}
                                  </span>
                                )}
                              </small>
                            )}
                          </article>
                        </Fragment>
                      );
                    })}
                    {working.map((id) => (
                      <WorkCard
                        key={id}
                        name={name(id)}
                        steps={liveHere[id] || []}
                        showName={!!room}
                      />
                    ))}
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
                  {commands.open && (
                    <ul className="mention-menu command-menu" id="mention-menu" role="listbox" aria-label="Commands">
                      {commands.options.map((o, i) => (
                        <li
                          key={o.kind + o.id}
                          id={"mention-" + i}
                          role="option"
                          aria-selected={i === commands.active}
                          onMouseDown={(e) => {
                            e.preventDefault();
                            pickCommand(o);
                          }}
                        >
                          <span className="all-icon">
                            {o.kind === "folder" ? <FolderOpen size={17} /> : o.kind === "model" ? <Cpu size={17} /> : <Slash size={17} />}
                          </span>
                          <span>
                            <strong>{o.label}</strong>
                            {o.detail && <small>{o.detail}</small>}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                  {mention.open && !commands.open && (
                    <ul className="mention-menu" id="mention-menu" role="listbox" aria-label="Mention a coworker">
                      {mention.options.map((p, i) => (
                        <li
                          key={p.id}
                          id={"mention-" + i}
                          role="option"
                          aria-selected={i === mention.active}
                          onMouseDown={(e) => {
                            e.preventDefault();
                            pickMention(p);
                          }}
                        >
                          {p.id === "all" ? (
                            <span className="all-icon"><AtSign size={18} /></span>
                          ) : (
                            <Avatar {...look(p.id)} size={30} />
                          )}
                          <span>
                            <strong>@{p.handle}</strong>
                            <small>{p.name} · {p.detail}</small>
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                  <form
                    className="composer"
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (
                        busyNow.current ||
                        (!(drafts[selected] || "").trim() && !attachment)
                      )
                        return;
                      if (commandAgent && !attachment && parseCommand(drafts[selected] || ""))
                        return runCommand(drafts[selected] || "");
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
                          setMessages((v) => mergeMessages(v, [m]));
                          throw Error(
                            "Your message was saved but could not be delivered. Your draft is kept so you can try again.",
                          );
                        }
                        nearBottom.current = true;
                        setMessages((v) => mergeMessages(v, [m]));
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
                        room
                          ? "Message the team"
                          : "Message or /help"
                      }
                      rows={1}
                      value={drafts[selected] || ""}
                      role="combobox"
                      aria-expanded={commands.open || mention.open}
                      aria-controls="mention-menu"
                      aria-autocomplete="list"
                      aria-activedescendant={
                        commands.open ? "mention-" + commands.active : mention.open ? "mention-" + mention.active : undefined
                      }
                      onChange={(e) => {
                        const value = e.currentTarget.value;
                        const conversation = selected;
                        setCaret(e.currentTarget.selectionStart ?? value.length);
                        setDrafts((v) => ({ ...v, [conversation]: value }));
                      }}
                      onSelect={(e) => setCaret(e.currentTarget.selectionStart ?? 0)}
                      onKeyDown={(e) => {
                        if (e.nativeEvent.isComposing || e.keyCode === 229)
                          return;
                        if (commands.open) {
                          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                            e.preventDefault();
                            commands.move(e.key === "ArrowDown" ? 1 : -1);
                            return;
                          }
                          if (e.key === "Enter" || e.key === "Tab") {
                            e.preventDefault();
                            pickCommand(commands.options[commands.active]);
                            return;
                          }
                          if (e.key === "Escape") {
                            e.preventDefault();
                            commands.dismiss();
                            return;
                          }
                        }
                        if (mention.open) {
                          if (e.key === "ArrowDown" || e.key === "ArrowUp") {
                            e.preventDefault();
                            mention.move(e.key === "ArrowDown" ? 1 : -1);
                            return;
                          }
                          if (e.key === "Enter" || e.key === "Tab") {
                            e.preventDefault();
                            pickMention(mention.options[mention.active]);
                            return;
                          }
                          if (e.key === "Escape") {
                            e.preventDefault();
                            mention.dismiss();
                            return;
                          }
                        }
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
                  <MaceMark className="welcome-mark" title="" />
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
                <h1>Your machines.</h1>
                <Machines
                  machines={[
                    {
                      id: machines.find((n) => n.self)?.id || "local",
                      name: machine.host || String(node.host_name || node.name || "This computer"),
                      self: true,
                      available: true,
                    },
                    ...peers.map((p) => ({
                      id: p.id,
                      name: p.name,
                      self: false,
                      available: machines.find((n) => n.id === p.id)?.available ?? true,
                      outdated: machines.find((n) => n.id === p.id)?.outdated,
                    })),
                  ]}
                  localHistory={machine.history}
                  agentIDs={[...agents, ...remote].filter((a) => a.status !== "removed").map((a) => a.id)}
                  agentRow={(id) => {
                    const a = [...agents, ...remote].find((x) => x.id === id);
                    return a ? agentRow(a) : null;
                  }}
                  run={(fn) => void action(fn)}
                  busy={busy}
                  onStart={startOn}
                  onPreview={(m, p) => {
                    setPreview(p);
                    setPreviewOn(m.self ? null : m);
                    setModal("preview");
                  }}
                  onLaunched={(m) => setNotice("Opening on " + m.name + ". Its desktop must be signed in.")}
                />
                <button className="control-row" onClick={() => nav("hosts")}>
                  Hosts connected by token <ChevronRight size={17} />
                </button>
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
                  Click an app to open it on your host desktop. A desktop session must be signed in; results appear in Activity.
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
                {Object.entries(
                  apps
                    .filter((a) => a.name.toLowerCase().includes(search.toLowerCase()))
                    .sort((a, b) => a.name.localeCompare(b.name))
                    .reduce<Record<string, typeof apps>>((groups, a) => {
                      (groups[a.category || "Other"] ||= []).push(a);
                      return groups;
                    }, {}),
                )
                  .sort(([a], [b]) => (a === "Other" ? 1 : b === "Other" ? -1 : a.localeCompare(b)))
                  .map(([category, list]) => (
                    <section className="app-group" key={category}>
                      <h2>{category}</h2>
                      <div className="app-grid">
                        {list.map((a) => (
                          <button key={a.id} title={"Open " + a.name} disabled={busy} onClick={() => void open(a.id)}>
                            <AppTile name={a.name} />
                            <span>{a.name}</span>
                          </button>
                        ))}
                      </div>
                    </section>
                  ))}
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
                    <ActivityBars times={activity.map((a) => a.at)} />
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
                              <EventIcon action={a.action} />
                              <span>{eventTitle(a.action)}</span>
                              <small>{actorLabel(a.actor, name)}</small>
                            </summary>
                            <EventDetails target={a.target} details={a.details} />
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
                        <small>{statusLabel(p.status)}</small>
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
                    <p className="peer-url">{p.url}</p>
                    {(() => {
                      const list = remote.filter((a) => a.id.startsWith(p.id + "::"));
                      return list.length ? (
                        <div className="peer-agents">
                          {list.map((a) => (
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
                        </div>
                      ) : (
                        <p className="muted">
                          No agents on this host yet. Start one from Your machines.
                        </p>
                      );
                    })()}
                  </section>
                ))}
              </>
            ) : page === "settings" ? (
              <>
                <h1>Settings.</h1>
                <section className="setting appearance">
                  <div>
                    <h2>Theme</h2>
                    <p>
                      Every theme comes from the MACE mark. Auto follows your
                      device between Signal and Ink.
                    </p>
                  </div>
                  <div className="theme-grid" role="radiogroup" aria-label="Theme">
                    {[
                      { id: "auto" as const, name: "Auto", note: "Follows your device." },
                      ...palettes,
                    ].map((p) => (
                      <button
                        key={p.id}
                        role="radio"
                        aria-checked={pref === p.id}
                        className="theme-card"
                        data-preview={p.id === "auto" ? resolve("auto") : p.id}
                        onClick={() => setPref(p.id)}
                      >
                        <span className="theme-preview" aria-hidden="true">
                          <i className="tp-rail" />
                          <i className="tp-in" />
                          <i className="tp-out" />
                          <i className="tp-in short" />
                        </span>
                        <strong>{p.name}</strong>
                        <small>{p.note}</small>
                      </button>
                    ))}
                  </div>
                </section>
                <section className="setting">
                  <div>
                    <h2>Keep working while I'm away</h2>
                    <p>
                      {keepWorking
                        ? "Agents keep going when every device is closed or locked."
                        : "Agents stop ten minutes after every device is closed or locked."}
                    </p>
                  </div>
                  <button
                    className="switch-track"
                    role="switch"
                    aria-checked={keepWorking}
                    aria-label="Keep working while I'm away"
                    onClick={() =>
                      void action(async () => {
                        const r = await api<{ keep_working: boolean }>("/v1/settings/presence", { keep_working: !keepWorking });
                        setKeepWorking(r.keep_working);
                      })
                    }
                  >
                    <span />
                  </button>
                </section>
                <section className="setting">
                  <div>
                    <h2>Chat wallpaper</h2>
                    <p>A quiet pattern of the MACE shapes behind conversations.</p>
                  </div>
                  <button
                    className="switch-track"
                    role="switch"
                    aria-checked={wallpaper}
                    aria-label="Chat wallpaper"
                    onClick={() => setWallpaperOn(!wallpaper)}
                  >
                    <span />
                  </button>
                </section>
                <section className="setting">
                  <div>
                    <h2>Notifications on this device</h2>
                    <p>
                      {!pushSupported()
                        ? "Notifications need the HTTPS address that mace-server start prints, or this computer's own browser."
                        : pushOn
                          ? "You hear about private replies, questions, blockers, plans, decisions, and finished work."
                          : "Get a notification when an agent needs you. Routine group chat stays quiet."}
                    </p>
                  </div>
                  <button
                    className="switch-track"
                    role="switch"
                    aria-checked={pushOn}
                    aria-label="Notifications on this device"
                    disabled={!pushSupported() || busy}
                    onClick={() => void action(togglePush)}
                  >
                    <span />
                  </button>
                </section>
                <section className="setting phone-signin">
                  <div>
                    <h2>Add a phone or tablet</h2>
                    <p>
                      {phoneLink
                        ? "Scan this with the other device's camera. It signs in once and stops working after ten minutes."
                        : "Devices on your Tailscale account sign in just by opening this hub. For any other device, show a code that signs it in without sharing your access token."}
                    </p>
                    {phoneLink && (
                      <figure className="signin-code">
                        <img
                          src={"data:image/svg+xml;utf8," + encodeURIComponent(phoneLink.svg)}
                          alt="Sign-in code for another device"
                          width={220}
                          height={220}
                        />
                        <figcaption>{phoneLink.link.replace(/#.*/, "")}</figcaption>
                      </figure>
                    )}
                  </div>
                  <button
                    className="secondary"
                    disabled={busy}
                    onClick={() =>
                      void action(async () =>
                        setPhoneLink(await api<{ link: string; svg: string }>("/v1/signin/qr")),
                      )
                    }
                  >
                    {phoneLink ? "New code" : "Show code"}
                  </button>
                </section>
                <section className="setting devices machines">
                  <div>
                    <h2>Your machines</h2>
                    <p>
                      Your other machines running MACE join by themselves
                      through your Tailscale account. Open one to switch to it,
                      already signed in.
                    </p>
                  </div>
                  <ul>
                    {machines.filter((n) => !n.self).map((n) => (
                      <li key={n.id}>
                        <span>
                          {n.name}
                          <small>{n.available ? "Available" : "Offline"}</small>
                        </span>
                        <button
                          className="secondary"
                          disabled={busy || !n.available}
                          onClick={() =>
                            void action(async () => {
                              const { url } = await api<{ url: string }>("/v1/nodes/" + n.id + "/open", {});
                              location.href = url;
                            })
                          }
                        >
                          Open
                        </button>
                      </li>
                    ))}
                    {!machines.some((n) => !n.self) && (
                      <li className="muted">
                        No other machines yet. Start MACE on another computer
                        logged in to your Tailscale account and it appears here.
                      </li>
                    )}
                  </ul>
                  <button className="secondary" onClick={() => nav("hosts")}>
                    Hosts connected by token
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
                    "Stop MACE",
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
                  <MaceMark className="intro-mark" title="" />
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
                    <p>Group agents from this machine or another MACE host.</p>
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
              role:'Role in this group',
              "new-session": "Start a fresh session?",
              profile: "Name, mention and role",
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
              <Media path={preview} base={previewOn ? machineBase(previewOn) : undefined} />
              {previewOn ? (
                <button
                  className="secondary"
                  onClick={() =>
                    void action(async () => {
                      const r = await fetch(machineBase(previewOn) + "/host/open", {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ path: full(preview) }),
                      });
                      if (!r.ok) throw Error((await r.text()).trim());
                      setNotice("Opening on " + previewOn.name + ".");
                    })
                  }
                >
                  Open on {previewOn.name}
                </button>
              ) : (
                <>
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
              )}
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
                  {room ? (
                    <>
                      <option value="team">Goal: a lead runs the team until done</option>
                      <option value="reply">Free: casual chat</option>
                      {(policy.mode === "goal" || policy.mode === "continuous") && (
                        <option value={policy.mode}>{policy.mode === "goal" ? "Goal oriented (older)" : "Until done (older)"}</option>
                      )}
                    </>
                  ) : (
                    <>
                      <option value="reply">Reply limit</option>
                      <option value="goal">Goal oriented</option>
                    </>
                  )}
                </select>
              </label>
              {room && policy.mode === "team" && (
                <label>
                  Lead
                  <select
                    required
                    value={policy.lead || ""}
                    onChange={(e) => setPolicy({ ...policy, lead: e.target.value })}
                  >
                    <option value="" disabled>
                      Choose the lead
                    </option>
                    {room.members.map((id) => (
                      <option key={id} value={id}>
                        {name(id)}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {room && (
                <label>
                  Texting style
                  <select value={policy.tone || "casual"} onChange={(e) => setPolicy({ ...policy, tone: e.target.value })}>
                    <option value="casual">Casual: short, chat-style, shorthand welcome</option>
                    <option value="professional">Professional: short and plain</option>
                  </select>
                </label>
              )}
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
              {policy.mode !== "continuous" && policy.mode !== "team" && (
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
                {policy.mode === "team"
                  ? "Your messages go to the lead unless you @mention someone. The lead plans, assigns roles, directs the others, and says done when everything works. Agents reach the lead with @lead."
                  : policy.mode === "continuous"
                  ? "After your message, agents keep working together until one reports it done, they run out of things to add, or you step in. They never start on their own."
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
              {!room && chat && all.some((a) => a.id === chat.id) && (
                <button
                  className="control-row"
                  onClick={() => {
                    const a = all.find((x) => x.id === chat.id)!;
                    setProfileDraft({
                      name: a.name,
                      handle: a.handle || "",
                      role: a.role || "",
                      color: profileFor(a.color, a.handle, a.id),
                    });
                    setModal("profile");
                  }}
                >
                  <Pencil size={18} />
                  Name, mention and role
                </button>
              )}
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
                        <button className="member-role" aria-label={'Assign role to '+name(id)} onClick={()=>{setRoleMember(id);setRoleDraft(room.roles?.[id]||'');setModal('role')}}>{room.roles?.[id]||(all.find(a=>a.id===id)?.role?'Own role: '+all.find(a=>a.id===id)!.role:'No assigned role')}<Pencil size={14}/></button>
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
          ) : modal === "profile" ? (
            <form
              className="profile-form"
              onSubmit={(event) => {
                event.preventDefault();
                void action(async () => {
                  await api("/v1/sessions/" + encodeURIComponent(selected) + "/profile", profileDraft);
                  await refresh();
                  setModal("");
                  setNotice("Saved. Every conversation uses this on the next turn.");
                });
              }}
            >
              <div className="profile-preview">
                <Avatar {...look(selected)} color={profileDraft.color} handle={profileDraft.handle} size={64} />
                <div>
                  <strong>{profileDraft.name || "Unnamed agent"}</strong>
                  <small>@{profileDraft.handle || "handle"}</small>
                </div>
              </div>
              <label>
                Name
                <input
                  autoFocus
                  required
                  maxLength={60}
                  value={profileDraft.name}
                  onChange={(e) => setProfileDraft({ ...profileDraft, name: e.currentTarget.value })}
                />
              </label>
              <label>
                Mention as
                <span className="prefixed">
                  <span aria-hidden="true">@</span>
                  <input
                    required
                    maxLength={32}
                    pattern="[A-Za-z0-9._\-]+"
                    title="Letters, numbers, dots, dashes, or underscores"
                    value={profileDraft.handle}
                    onChange={(e) => setProfileDraft({ ...profileDraft, handle: e.currentTarget.value.replace(/\s+/g, "-") })}
                  />
                </span>
              </label>
              <label>
                Role
                <textarea
                  rows={3}
                  maxLength={2000}
                  placeholder="For example, review every change for security"
                  value={profileDraft.role}
                  onChange={(e) => setProfileDraft({ ...profileDraft, role: e.currentTarget.value })}
                />
              </label>
              <p className="hint">
                This role goes with the agent into every group. A group can give it a different role there.
              </p>
              <fieldset className="swatches">
                <legend>Color</legend>
                {colorProfiles.map((label, i) => (
                  <button
                    type="button"
                    key={label}
                    className={"swatch profile-" + i}
                    aria-label={label}
                    aria-pressed={profileDraft.color === i}
                    onClick={() => setProfileDraft({ ...profileDraft, color: i })}
                  />
                ))}
              </fieldset>
              <button className="primary" disabled={busy}>
                Save
              </button>
            </form>
          ) : modal === "new-session" ? (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                setModal("");
                runCommand("/new", true);
              }}
            >
              <p>
                {commandAgent?.name || "The agent"} starts its next turn in a new native session. Messages here stay, but it
                will not remember its own earlier work, files read, or tool calls.
              </p>
              <button className="primary" autoFocus disabled={busy}>
                Start fresh
              </button>
            </form>
          ) : modal === "role" ? (
            <form onSubmit={event=>{event.preventDefault();void action(async()=>{await api('/v1/rooms/'+selected+'/members/'+encodeURIComponent(roleMember)+'/role',{role:roleDraft});await refresh();setModal('details')})}}><p>{all.find(a=>a.id===roleMember)?.role?<>In other conversations {name(roleMember)} works as: {all.find(a=>a.id===roleMember)!.role}. A role set here applies only in this group.</>:<>{name(roleMember)} is unassigned unless you give them a role. Coworkers can also find responsibilities naturally while they work.</>}</p><label>Role (optional)<input autoFocus value={roleDraft} maxLength={2000} placeholder="For example, review the mobile layouts" onChange={event=>setRoleDraft(event.currentTarget.value)}/></label><p>Clear the field to leave this coworker unassigned. Changes apply on their next turn.</p><button className="primary" disabled={busy}>Save role</button></form>
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
function Media({ path, base = "/api/v1", onFolder }: { path: string; base?: string; onFolder?: (path: string) => void }) {
  if (path.endsWith("/")) {
    // A shared folder: open it in Files rather than previewing it.
    const name = path.replace(/\/+$/, "").split("/").pop() || "/";
    return (
      <button className="media folder-share" onClick={() => onFolder?.(path.replace(/\/+$/, ""))}>
        <Folder size={22} />
        <span>
          <strong>{name}/</strong>
          <small>Shared folder · open in Files</small>
        </span>
        <ChevronRight size={17} />
      </button>
    );
  }
  const url = base + "/files?path=" + encodeURIComponent(path),
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
