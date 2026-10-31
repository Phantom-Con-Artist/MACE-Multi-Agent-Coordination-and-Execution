import { useState } from "react";
import {
  ArrowDown,
  ArrowRight,
  ArrowUpRight,
  AtSign,
  Brain,
  Eye,
  PauseCircle,
  ShieldCheck,
  Users,
  WifiOff,
} from "lucide-react";
import { HyperbolicDisk, StepArt, TwoHosts } from "./art";
import { MaceLogo } from "./brand";
import { Avatar } from "./messenger-parts";

const steps = [
  {
    title: "Start your host.",
    detail:
      "Run one command on the computer that should do the work. MACE prints a sign-in link and keeps running in the background.",
    note: "mace start",
  },
  {
    title: "Open the link.",
    detail:
      "Open the link on your phone or laptop and you are signed in. On another device, use the network address MACE prints.",
    note: "Any screen with a browser",
  },
  {
    title: "Pick an agent. Talk.",
    detail:
      "Tap + and choose an agent installed on your computer. Ask for what you need. Add more agents to a group when the work grows.",
    note: "No forms, no flags",
  },
];

const coworkers = [
  {
    icon: Users,
    title: "Everyone answers at once.",
    text: "Write to a group and every agent starts working in parallel, each with its own tools.",
  },
  {
    icon: AtSign,
    title: "Mention to hand off.",
    text: "@muse gives Muse the next turn. Agents mention each other the same way to ask for help.",
  },
  {
    icon: Brain,
    title: "One memory everywhere.",
    text: "An agent remembers your private chat when it joins a group, and the group when you talk privately.",
  },
  {
    icon: Eye,
    title: "Watch the work.",
    text: "Each file it reads, each edit, each command appears as it happens, then folds away under the reply.",
  },
];

const safety = [
  {
    icon: WifiOff,
    title: "Stops when you step away.",
    text: "If every device you signed in with goes quiet, agents stop. Continue picks up where they left off.",
  },
  {
    icon: PauseCircle,
    title: "Pause everything.",
    text: "One tap stops every agent on this computer and on every computer you connected.",
  },
  {
    icon: ShieldCheck,
    title: "Your token stays yours.",
    text: "Agents and the apps they start never receive your MACE access token.",
  },
];

function Command() {
  const [copied, setCopied] = useState(false);
  return (
    <button
      className="cmd"
      onClick={() =>
        void navigator.clipboard
          ?.writeText("mace start")
          .then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1600);
          })
          .catch(() => {})
      }
      aria-label="Copy the command mace start"
    >
      <span className="cmd-prompt">$</span>
      <code>mace start</code>
      <span className="cmd-state">{copied ? "Copied" : "Copy"}</span>
    </button>
  );
}

export default function Landing({ go }: { go: (page: string) => void }) {
  const [step, setStep] = useState(0);
  const smooth = () =>
    matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";
  return (
    <main className="landing2">
      <section className="l-hero">
        <div className="l-hero-copy">
          <p className="eyebrow">Multi Agent Coordination and Execution</p>
          <h1>
            Your agents.
            <br />
            <span className="accent-word">One</span> conversation.
          </h1>
          <p className="lede">
            MACE runs the command-line agents already on your computer. Talk to
            one, or put a few in a group and let them work together. From your
            phone, or any screen.
          </p>
          <div className="l-actions">
            <button className="primary big" onClick={() => go("messenger")}>
              Open workspace <ArrowUpRight size={18} />
            </button>
            <Command />
          </div>
          <button
            className="l-more"
            onClick={() =>
              document.getElementById("start")?.scrollIntoView({ behavior: smooth() })
            }
          >
            How it works <ArrowDown size={16} />
          </button>
        </div>
        <div className="l-hero-art">
          <HyperbolicDisk label="A hyperbolic tiling of red and black triangles that repeat toward the rim" />
        </div>
      </section>

      <section className="l-agents" aria-label="Supported agents">
        <p>Bring the agents you already use</p>
        <ul>
          {[
            ["claude", "Claude Code"],
            ["codex", "Codex"],
            ["agy", "Antigravity"],
            ["muse", "Muse"],
          ].map(([id, label]) => (
            <li key={id}>
              <Avatar id={"site-" + id} provider={id} size={36} />
              {label}
            </li>
          ))}
        </ul>
      </section>

      <section className="l-steps" id="start">
        <header className="l-head">
          <span className="index">01</span>
          <h2>
            Three steps.
            <br />
            Then just talk.
          </h2>
        </header>
        <div className="l-steps-body">
          <ol role="tablist" aria-label="Getting started">
            {steps.map((s, i) => (
              <li key={s.title}>
                <button
                  role="tab"
                  aria-selected={step === i}
                  aria-controls="step-panel"
                  onClick={() => setStep(i)}
                >
                  <span className="n">{i + 1}</span>
                  <span>
                    <strong>{s.title}</strong>
                    <small>{s.detail}</small>
                    <em>{s.note}</em>
                  </span>
                </button>
              </li>
            ))}
          </ol>
          <div id="step-panel" role="tabpanel" className="l-step-art" key={step}>
            <StepArt step={step} />
          </div>
        </div>
      </section>

      <section className="l-coworkers">
        <header className="l-head">
          <span className="index">02</span>
          <h2>
            Coworkers,
            <br />
            not chatbots.
          </h2>
        </header>
        <div className="l-grid">
          {coworkers.map(({ icon: Icon, title, text }) => (
            <article key={title}>
              <Icon size={22} />
              <h3>{title}</h3>
              <p>{text}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="l-safety">
        <header className="l-head">
          <span className="index">03</span>
          <h2>
            Safe when you
            <br />
            look away.
          </h2>
        </header>
        <div className="l-safety-list">
          {safety.map(({ icon: Icon, title, text }) => (
            <article key={title}>
              <Icon size={22} />
              <div>
                <h3>{title}</h3>
                <p>{text}</p>
              </div>
            </article>
          ))}
        </div>
      </section>

      <section className="l-hosts">
        <div>
          <span className="index">04</span>
          <h2>
            Another computer.
            <br />
            The same conversation.
          </h2>
          <p>
            Run MACE on a second computer and connect it. Its agents can join
            your groups, and attached files travel between the two with a
            checksum check.
          </p>
          <button className="l-link" onClick={() => go("manual")}>
            How connected computers work <ArrowRight size={16} />
          </button>
        </div>
        <TwoHosts />
      </section>

      <section className="l-help">
        {[
          ["manual", "The manual", "Your first chat, groups, files, and controls."],
          ["troubleshoot", "Something not working?", "Connection, agent, and file answers."],
          ["docs", "Under the surface", "Commands, settings, and the protocol."],
        ].map(([id, title, desc]) => (
          <button key={id} onClick={() => go(id)}>
            <span>
              <strong>{title}</strong>
              <small>{desc}</small>
            </span>
            <ArrowUpRight size={20} />
          </button>
        ))}
      </section>

      <section className="l-final">
        <MaceLogo inverse className="final-logo" title="MACE" />
        <h2>Pick an agent. Tell it what needs doing.</h2>
        <button className="primary big" onClick={() => go("messenger")}>
          Open workspace <ArrowUpRight size={18} />
        </button>
      </section>
    </main>
  );
}
