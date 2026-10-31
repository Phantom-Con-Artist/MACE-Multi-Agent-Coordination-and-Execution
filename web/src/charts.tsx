// Small SVG charts drawn in theme colors. A charting library would add far
// more weight than a phone needs for gauges, sparklines, and a bar strip.
import { useEffect, useState } from "react";
import { Activity, AppWindow, Bot, Cpu, FileText, Gauge, HardDrive, MemoryStick, MessageSquare, MonitorCog, Network, Pause, Terminal, Users } from "lucide-react";

export type Sample = {
  at: string;
  cpu: number;
  mem_used: number;
  mem_total: number;
  disk: number;
  load: number;
  cores: number;
  gpu?: number;
  gpu_mem?: number;
  gpu_name?: string;
};

const pct = (v: number) => Math.max(0, Math.min(100, v));
const gib = (b: number) => (b / 1024 ** 3).toFixed(1);

/** A ring gauge. Above 85% the ring turns to the signal color. */
export function Ring({ value, label, detail, icon: Icon }: { value: number; label: string; detail: string; icon: typeof Cpu }) {
  const r = 26,
    c = 2 * Math.PI * r,
    v = pct(value);
  return (
    <figure className={"gauge" + (v >= 85 ? " hot" : "")} aria-label={`${label} ${Math.round(v)} percent, ${detail}`}>
      <svg viewBox="0 0 64 64" aria-hidden="true">
        <circle cx="32" cy="32" r={r} className="gauge-track" />
        <circle
          cx="32"
          cy="32"
          r={r}
          className="gauge-value"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - v / 100)}
        />
      </svg>
      <span className="gauge-center">
        <Icon size={15} aria-hidden="true" />
        <b>{Math.round(v)}%</b>
      </span>
      <figcaption>
        <strong>{label}</strong>
        <small>{detail}</small>
      </figcaption>
    </figure>
  );
}

/** A sparkline of the last samples, with a soft area under the line. */
export function Spark({ values, label }: { values: number[]; label: string }) {
  const w = 240,
    h = 48;
  if (values.length < 2) return <div className="spark empty">Collecting…</div>;
  const step = w / (values.length - 1);
  const pts = values.map((v, i) => `${(i * step).toFixed(1)},${(h - (pct(v) / 100) * (h - 4) - 2).toFixed(1)}`);
  return (
    <svg className="spark" viewBox={`0 0 ${w} ${h}`} preserveAspectRatio="none" role="img" aria-label={label}>
      <polygon className="spark-area" points={`0,${h} ${pts.join(" ")} ${w},${h}`} />
      <polyline className="spark-line" points={pts.join(" ")} />
    </svg>
  );
}

/** One machine: gauges for now, a sparkline for the last ten minutes. */
export function MachineCard({ name, history, error }: { name: string; history: Sample[]; error?: string }) {
  const s = history.at(-1);
  return (
    <article className="machine">
      <header>
        <span className={"dot " + (s && !error ? "on" : "off")} aria-hidden="true" />
        <strong>{name}</strong>
        <small>{error ? "Unreachable" : s ? `${s.cores} cores · load ${s.load.toFixed(2)}` : "Starting…"}</small>
      </header>
      {error ? (
        <p className="machine-error">{error}</p>
      ) : s ? (
        <>
          <div className="gauges">
            <Ring value={s.cpu} label="CPU" detail={`${s.cores} cores`} icon={Cpu} />
            <Ring
              value={(s.mem_used / s.mem_total) * 100}
              label="Memory"
              detail={`${gib(s.mem_used)} of ${gib(s.mem_total)} GiB`}
              icon={MemoryStick}
            />
            {s.gpu !== undefined ? (
              <Ring value={s.gpu} label="GPU" detail={s.gpu_mem !== undefined ? `${Math.round(s.gpu_mem)}% memory` : s.gpu_name || "Busy"} icon={MonitorCog} />
            ) : (
              <Ring value={(s.load / Math.max(1, s.cores)) * 100} label="Load" detail={`${s.load.toFixed(2)} of ${s.cores}`} icon={Gauge} />
            )}
            <Ring value={s.disk} label="Disk" detail="Root filesystem" icon={HardDrive} />
          </div>
          <div className="machine-trend">
            <small>
              CPU, last {Math.max(1, Math.round((history.length * 5) / 60))} min · peak {Math.round(Math.max(...history.map((h) => h.cpu)))}%
            </small>
            <Spark values={history.map((h) => h.cpu)} label="CPU over the last minutes" />
          </div>
        </>
      ) : (
        <p className="machine-error">Waiting for the first reading.</p>
      )}
    </article>
  );
}

/** Polls a connected host's monitor while visible. */
export function usePeerMetrics(peerID: string, active: boolean) {
  const [state, setState] = useState<{ history: Sample[]; error?: string }>({ history: [] });
  useEffect(() => {
    if (!active) return;
    let alive = true;
    const load = () =>
      fetch("/api/v1/peers/" + encodeURIComponent(peerID) + "/metrics")
        .then(async (r) => {
          if (!r.ok) throw Error((await r.text()).trim() || "Unavailable");
          return r.json();
        })
        .then((d) => alive && setState({ history: d.history || [] }))
        .catch((e) => alive && setState((s) => ({ ...s, error: String(e.message || e) })));
    void load();
    const id = setInterval(load, 5000);
    return () => {
      alive = false;
      clearInterval(id);
    };
  }, [peerID, active]);
  return state;
}

// Audit times come from Go as "2006-01-02 15:04:05.999999999 +0000 UTC".
const parseTime = (t: string) =>
  new Date(t.includes("T") ? t : t.replace(" +0000 UTC", "Z").replace(" ", "T").replace(/(\.\d{3})\d+/, "$1")).getTime();

/** Events per hour for the last day, as a bar strip. */
export function ActivityBars({ times }: { times: string[] }) {
  const now = Date.now();
  const buckets = Array.from({ length: 24 }, () => 0);
  for (const t of times) {
    const age = Math.floor((now - parseTime(t)) / 3600000);
    if (age >= 0 && age < 24) buckets[23 - age]++;
  }
  const max = Math.max(1, ...buckets);
  return (
    <figure className="activity-bars" aria-label={`${times.length} events; busiest hour had ${max}`}>
      <svg viewBox="0 0 240 64" preserveAspectRatio="none" aria-hidden="true">
        {buckets.map((n, i) => (
          <rect key={i} x={i * 10 + 1} width="8" y={64 - (n / max) * 60} height={(n / max) * 60} rx="2" className={n ? "bar" : "bar empty"} />
        ))}
      </svg>
      <figcaption>
        <span>24 hours ago</span>
        <span>Now</span>
      </figcaption>
    </figure>
  );
}

/** "CONVERSATION_COMPLETED" reads as "Conversation completed". */
export const eventTitle = (action: string) => {
  const t = action.replaceAll("_", " ").toLowerCase();
  return t.charAt(0).toUpperCase() + t.slice(1);
};

// Host-side actors are internal parts of MACE; show them as MACE.
const hostActors = new Set(["mace", "orchestrator", "supervisor"]);
export const actorLabel = (actor: string, name: (id: string) => string) =>
  hostActors.has(actor) ? "MACE" : actor === "peer" ? "Connected host" : name(actor);

/** A small mark on the timeline rail for each kind of event. */
export function EventIcon({ action }: { action: string }) {
  const a = action.toUpperCase();
  const Icon = a.startsWith("FILE")
    ? FileText
    : a.startsWith("PAUSE") || a.includes("_PAUSE")
      ? Pause
      : a.startsWith("PROCESS")
        ? Terminal
        : a.startsWith("HOST")
          ? AppWindow
          : a.startsWith("PEER") || a.startsWith("REMOTE")
            ? Network
            : a.startsWith("GROUP") || a.startsWith("MEMBER")
              ? Users
              : a.startsWith("AGENT") || a.startsWith("START")
              ? Bot
              : a.startsWith("CONVERSATION") || a.startsWith("TURN") || a.startsWith("MESSAGE") || a.startsWith("SESSION")
                ? MessageSquare
                : Activity;
  return (
    <span className="event-mark" aria-hidden="true">
      <Icon size={14} />
    </span>
  );
}

/** An event's target and details as a wrapped list, readable on a phone. */
export function EventDetails({ target, details }: { target?: string; details?: unknown }) {
  const extra = details && typeof details === "object" ? (details as Record<string, unknown>) : { details };
  const rows = Object.entries({ target, ...extra }).filter(([, v]) => v !== undefined && v !== null && v !== "");
  if (!rows.length) return <p className="event-details muted">No further details.</p>;
  return (
    <dl className="event-details">
      {rows.map(([k, v]) => (
        <div key={k}>
          <dt>{eventTitle(k)}</dt>
          <dd>{typeof v === "object" ? JSON.stringify(v) : String(v)}</dd>
        </div>
      ))}
    </dl>
  );
}
