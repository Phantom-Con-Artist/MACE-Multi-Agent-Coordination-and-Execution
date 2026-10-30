import { useEffect, useRef, useState } from "react";
import {
  ArrowRight,
  ArrowDown,
  Copy,
  Check,
  Menu,
  X,
  Sun,
  Moon,
  ArrowUpRight,
  Terminal,
  MessageCircle,
  Folder,
  Monitor,
  BookOpen,
  LifeBuoy,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Accordion,
  AccordionItem,
  AccordionTrigger,
  AccordionContent,
} from "@/components/ui/accordion";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Separator } from "@/components/ui/separator";
import "./public-site.css";

const sections = [
  ["home", "Home"],
  ["manual", "Manual"],
  ["docs", "Docs"],
  ["troubleshoot", "Troubleshoot"],
  ["about", "About"],
];
const steps = [
  {
    title: "Start your host.",
    detail:
      "On the computer you want to work with, run one command. Mace prints your address, access token, and a sign-in link.",
    command: "mace start",
    action: "Keep this computer running while you work remotely.",
  },
  {
    title: "Open the link.",
    detail:
      "Open the printed sign-in link in your browser. On another device, use the printed network address. The link connects you automatically.",
    command: "Open your sign-in link",
    action: "Your phone or laptop needs a network route to the host.",
  },
  {
    title: "Choose your agent.",
    detail:
      "Tap + in Chats and choose an installed agent. Start in root, or pick a folder. Tell it what you would like done.",
    command: "Chats → + → Choose an agent",
    action: "No launch arguments or configuration form to fill out.",
  },
];
const guide = [
  [
    "Your first conversation",
    "Open the workspace, tap + in Chats, and choose an installed agent. Mace looks for Codex, Claude Code, Antigravity, and Muse. Each agent uses its existing login and permissions on the host. New agents start in root /. Choose Change to browse to a different folder before starting.",
  ],
  [
    "Choose where an agent works",
    "Use the folder picker before starting an agent. You can also browse Host files and choose Start agent here. The folder determines where the native CLI runs. Access is limited by the host operating system and the account running Mace.",
  ],
  [
    "Put agents in a group",
    "Use New group, give it a name, and choose agents. Agents take turns with the shared conversation context. Local agents use the same host filesystem, so a path is enough to share a file. You can add or remove members from Conversation controls.",
  ],
  [
    "Set a stopping point",
    "Reply limit sets the total number of agent replies. Goal oriented mode asks agents to work toward your goal, then stops on their completion report or the reply limit. Continuous mode runs until paused or a reply fails. A completion report is a claim to review, not a guarantee.",
  ],
  [
    "Pause, stop, or remove an agent",
    "Pause stops the current conversation run. Stop cancels an agent’s current work. Terminate makes the agent unavailable until resumed. Remove takes it out of your chat list and groups while preserving history. These controls are inside the conversation menu and on the Host page.",
  ],
  [
    "Open files and apps",
    "Host files lets you browse folders, read text, preview pictures, and play supported audio or video. Open an app launches an installed application on the host desktop. Mace does not stream its window into your browser. You can inspect launch results and process output in Activity.",
  ],
  [
    "Connect another Mace host",
    "Run Mace on the other computer, then open Connected hosts and enter its address and access token. Its agents can join your groups. Attachments are transferred with a checksum check, up to 32 MiB per file. Both hosts need a working network connection.",
  ],
];
const issues = [
  [
    "I cannot connect to my host",
    "Run mace status on the host. If it is stopped, run mace start and use the new printed sign-in link. On the host itself use the local address. On a different device use the network address, not 127.0.0.1. Check that both devices can reach each other and that the host firewall permits the configured port.",
  ],
  [
    "The access token does not work",
    "Use the sign-in link printed by the currently running server. A manually configured MACE_AUTH_SECRET overrides the saved token. If you change it, stop and restart Mace, then open its new link. The token belongs to the host you are connecting to.",
  ],
  [
    "My agent is not in the list",
    "Install a supported CLI on the host and check that its executable is on the PATH used to start Mace. Finish its normal sign-in process on the host. Restart Mace if you changed PATH. The Configuration page shows which supported agents are detected.",
  ],
  [
    "An agent failed or did not reply",
    "Open Activity and inspect its native process log. Provider login, permissions, network access, and usage limits come from the installed CLI. Resume a stopped or terminated agent before sending a new task. A failed conversation stays visible with its error.",
  ],
  [
    "An app did not open",
    "Desktop apps open on the host, not your remote screen. The host needs an active graphical session and its desktop-launch tools. Open Activity to see whether the launch command exited with an error. Headless servers may have no desktop session.",
  ],
  [
    "A file will not play or transfer",
    "Media playback depends on browser codec support. Download an unsupported format and open it with a suitable app. Cross-host transfers currently support files up to 32 MiB. Check file permissions and whether the receiving host can write to its inbox.",
  ],
  [
    "A conversation keeps going",
    "Use Pause in the conversation header. Continuous mode deliberately runs until paused or a reply fails. For a bounded task choose Reply limit or Goal oriented mode before sending your next message.",
  ],
];
function Art({ step = 0 }: { step?: number }) {
  return (
    <div className="setup-art">
      <img
        src="/art/mace-setup.png"
        style={{ transform: `translateX(-${(step * 100) / 3}%)` }}
        alt={
          [
            "Illustration of starting a host computer",
            "Illustration of a phone connecting to a laptop",
            "Illustration of three geometric agents talking",
          ][step]
        }
        loading="lazy"
      />
    </div>
  );
}
function CopyCommand() {
  const [copied, setCopied] = useState(false),
    [failed, setFailed] = useState(false);
  return (
    <div className="command-copy">
      <Terminal size={18} />
      <code>mace start</code>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Copy start command"
        onClick={async () => {
          try {
            await navigator.clipboard.writeText("mace start");
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
          } catch {
            setFailed(true);
          }
        }}
      >
        {copied ? <Check size={17} /> : <Copy size={17} />}
      </Button>
      {failed && <span role="status">Select the command to copy it.</span>}
    </div>
  );
}
export default function PublicSite({
  page,
  navigate,
  dark,
  toggleTheme,
  connected,
}: {
  page: string;
  navigate: (page: string) => void;
  dark: boolean;
  toggleTheme: () => void;
  connected: boolean;
}) {
  const [menu, setMenu] = useState(false),
    [query, setQuery] = useState("");
  const scroll = useRef<HTMLDivElement>(null);
  useEffect(() => {
    scroll.current?.scrollTo({ top: 0 });
    setMenu(false);
  }, [page]);
  const go = (id: string) => {
    setMenu(false);
    navigate(id);
  };
  return (
    <div className="public-site" ref={scroll}>
      <header className="site-header">
        <a className="site-brand" href="#home">
          <img src="/mace-mark.svg" alt="" />
          <span>MACE</span>
        </a>
        <nav
          aria-label="Website navigation"
          className={menu ? "site-nav expanded" : "site-nav"}
        >
          {sections.slice(1).map(([id, label]) => (
            <a
              key={id}
              href={"#" + id}
              aria-current={page === id ? "page" : undefined}
              onClick={() => setMenu(false)}
            >
              {label}
            </a>
          ))}
          <Button
            className="mobile-nav-theme"
            variant="ghost"
            onClick={toggleTheme}
          >
            {dark ? <Sun size={18} /> : <Moon size={18} />}Switch to{" "}
            {dark ? "light" : "dark"}
          </Button>
        </nav>
        <div className="site-header-actions">
          <Button
            variant="ghost"
            size="icon"
            aria-label="Toggle theme"
            onClick={toggleTheme}
          >
            {dark ? <Sun size={18} /> : <Moon size={18} />}
          </Button>
          <Button
            className="site-workspace-button"
            onClick={() => go("messenger")}
          >
            {connected ? "Your workspace" : "Open workspace"}
            <ArrowUpRight size={16} />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="mobile-site-menu"
            aria-label={menu ? "Close navigation" : "Open navigation"}
            onClick={() => setMenu(!menu)}
          >
            {menu ? <X /> : <Menu />}
          </Button>
        </div>
      </header>
      {page === "home" ? (
        <>
          <main className="landing">
            <section className="site-hero">
              <div className="hero-copy">
                <span className="product-longname">
                  Multi Agent Coordination and Execution
                </span>
                <h1>
                  Your computer.
                  <br />A little closer.
                </h1>
                <p>
                  Chat with your agents, open your files, and get things done on
                  your own computer. From the screen in front of you.
                </p>
                <div className="hero-actions">
                  <Button size="lg" onClick={() => go("messenger")}>
                    Open workspace <ArrowUpRight size={19} />
                  </Button>
                  <Button
                    variant="ghost"
                    onClick={() =>
                      document
                        .getElementById("getting-started")
                        ?.scrollIntoView({
                          behavior: matchMedia(
                            "(prefers-reduced-motion: reduce)",
                          ).matches
                            ? "auto"
                            : "smooth",
                        })
                    }
                  >
                    See how it works <ArrowDown size={16} />
                  </Button>
                </div>
                <div className="hero-command">
                  <span>Start on your host with</span>
                  <CopyCommand />
                </div>
              </div>
              <figure className="hero-art">
                <img
                  src="/art/mace-workspace.png"
                  alt="A geometric desktop computer and phone connected through a folded arch, with three agent shapes"
                  fetchPriority="high"
                  width="1536"
                  height="1024"
                />
                <figcaption>
                  <span className="art-label">Your computer stays home.</span>
                  <span className="art-label">Your work comes with you.</span>
                </figcaption>
              </figure>
            </section>
            <div className="compatibility">
              <span>Bring the agents you already use</span>
              <div>
                <strong>Codex</strong>
                <strong>Claude Code</strong>
                <strong>Antigravity</strong>
                <strong>Muse</strong>
              </div>
              <span>Runs on your host. Opens in your browser.</span>
            </div>
            <section className="getting-started" id="getting-started">
              <div className="section-title">
                <div>
                  <span className="section-index">01 / GETTING STARTED</span>
                  <h2>
                    Three small steps.
                    <br />
                    Then just chat.
                  </h2>
                </div>
                <p>
                  No launch forms to learn.
                  <br />
                  No terminal flags to remember.
                </p>
              </div>
              <Tabs defaultValue="0" className="setup-tabs">
                <TabsList className="setup-step-nav">
                  {steps.map((s, i) => (
                    <TabsTrigger key={i} value={String(i)}>
                      <span>{String(i + 1).padStart(2, "0")}</span>
                      {s.title}
                      <ArrowRight size={17} />
                    </TabsTrigger>
                  ))}
                </TabsList>
                {steps.map((s, i) => (
                  <TabsContent
                    key={i}
                    value={String(i)}
                    className="setup-content"
                  >
                    <Art step={i} />
                    <div className="setup-description">
                      <span className="small-caps">STEP {i + 1} OF 3</span>
                      <h3>{s.title}</h3>
                      <p>{s.detail}</p>
                      {i === 0 ? (
                        <CopyCommand />
                      ) : (
                        <div className="step-instruction">
                          {s.command}
                          <ArrowUpRight size={18} />
                        </div>
                      )}
                      <small>{s.action}</small>
                    </div>
                  </TabsContent>
                ))}
              </Tabs>
            </section>
            <section className="work-section">
              <div className="section-title">
                <div>
                  <span className="section-index">02 / YOUR WORKSPACE</span>
                  <h2>
                    A familiar place
                    <br />
                    to ask for a hand.
                  </h2>
                </div>
                <p>
                  Start with a conversation.
                  <br />
                  The controls are there when you need them.
                </p>
              </div>
              <div className="work-story">
                <div className="story-shape">
                  <div className="geometric-agent">
                    <div />
                    <div />
                    <div />
                  </div>
                  <span>ONE AGENT OR A SMALL TEAM</span>
                </div>
                <div className="work-explanations">
                  <article>
                    <MessageCircle size={23} />
                    <div>
                      <h3>Talk to your agents.</h3>
                      <p>
                        Choose an installed CLI and start a conversation. Bring
                        others into a group, give them a goal, or set a reply
                        limit.
                      </p>
                    </div>
                  </article>
                  <article>
                    <Folder size={23} />
                    <div>
                      <h3>Reach your files and apps.</h3>
                      <p>
                        Browse folders, inspect documents, play media, or open
                        an app on your host. Start an agent in the folder you
                        are looking at.
                      </p>
                    </div>
                  </article>
                  <article>
                    <ActivityIcon />
                    <div>
                      <h3>See what happened.</h3>
                      <p>
                        Follow the conversation, inspect process logs, and track
                        file transfers. Pause work or stop an agent at any
                        point.
                      </p>
                    </div>
                  </article>
                </div>
              </div>
            </section>
            <section className="two-host-story">
              <div>
                <span className="section-index">
                  03 / ROOM TO WORK TOGETHER
                </span>
                <h2>
                  Another computer.
                  <br />
                  The same conversation.
                </h2>
                <p>
                  Connect a second Mace host and bring its agents into your
                  groups. Local agents use shared paths. Files sent to another
                  host travel through Mace.
                </p>
                <Button variant="outline" onClick={() => go("manual")}>
                  How connected hosts work <ArrowRight size={17} />
                </Button>
              </div>
              <div
                className="host-pair"
                aria-label="Illustration of two connected computers"
              >
                <div>
                  <Monitor size={56} strokeWidth={1.5} />
                  <span>Your host</span>
                </div>
                <span className="host-connection" />
                <div>
                  <Monitor size={56} strokeWidth={1.5} />
                  <span>Another host</span>
                </div>
                <div className="shared-file">
                  <Folder size={21} />A file, shared deliberately.
                </div>
              </div>
            </section>
            <section className="site-help">
              <div>
                <LifeBuoy size={28} />
                <h2>
                  A little help,
                  <br />
                  when you need it.
                </h2>
                <p>
                  Short instructions, clear answers, and the details when you
                  want them.
                </p>
              </div>
              <div className="help-links">
                {[
                  [
                    "manual",
                    "The manual",
                    "Your first chat, groups, files, and host controls.",
                  ],
                  [
                    "troubleshoot",
                    "Something not working?",
                    "Connection, agent, app, and playback answers.",
                  ],
                  [
                    "docs",
                    "Under the surface",
                    "Configuration and protocol reference.",
                  ],
                ].map(([id, title, desc]) => (
                  <button key={id} onClick={() => go(id)}>
                    <span>
                      <strong>{title}</strong>
                      <small>{desc}</small>
                    </span>
                    <ArrowUpRight size={22} />
                  </button>
                ))}
              </div>
            </section>
            <section className="last-call">
              <img src="/mace-mark.svg" alt="" />
              <h2>
                Pick an agent.
                <br />
                Tell it what needs doing.
              </h2>
              <Button size="lg" onClick={() => go("messenger")}>
                Open workspace <ArrowUpRight size={19} />
              </Button>
            </section>
          </main>
        </>
      ) : (
        <main className="knowledge">
          <div className="knowledge-heading">
            <span className="section-index">
              MACE / {page === "troubleshoot" ? "HELP" : page.toUpperCase()}
            </span>
            <h1>
              {
                (
                  {
                    manual: "A little direction.",
                    docs: "The useful details.",
                    troubleshoot: "Let’s get it working.",
                    about: "Your computer. Your call.",
                  } as Record<string, string>
                )[page]
              }
            </h1>
            <p>
              {
                (
                  {
                    manual:
                      "From the first connection to a group of agents working together.",
                    docs: "Configuration and protocols, kept separate from your everyday work.",
                    troubleshoot:
                      "Start with what you are seeing. There is usually a small next step.",
                    about:
                      "MACE, Multi Agent Coordination and Execution. Version 0.1.0.",
                  } as Record<string, string>
                )[page]
              }
            </p>
          </div>
          {page === "manual" ? (
            <>
              <div className="manual-start">
                <Art />
                <div>
                  <h2>Start here.</h2>
                  <p>
                    Run this on the computer you want to work with. Open the
                    sign-in link it prints, then choose an agent in Chats.
                  </p>
                  <CopyCommand />
                  <Button variant="link" onClick={() => go("messenger")}>
                    Go to workspace <ArrowRight size={17} />
                  </Button>
                </div>
              </div>
              <Accordion
                type="single"
                collapsible
                className="knowledge-accordion"
              >
                {guide.map(([title, text], i) => (
                  <AccordionItem value={String(i)} key={title}>
                    <AccordionTrigger>
                      <span className="accordion-number">
                        {String(i + 1).padStart(2, "0")}
                      </span>
                      {title}
                    </AccordionTrigger>
                    <AccordionContent>
                      <p>{text}</p>
                    </AccordionContent>
                  </AccordionItem>
                ))}
              </Accordion>
            </>
          ) : page === "troubleshoot" ? (
            <>
              <label className="help-search">
                <Search size={19} />
                <input
                  aria-label="Search troubleshooting"
                  placeholder="Find an answer"
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                />
              </label>
              <Accordion
                type="single"
                collapsible
                className="knowledge-accordion"
              >
                {issues
                  .filter((i) =>
                    i.join(" ").toLowerCase().includes(query.toLowerCase()),
                  )
                  .map(([title, text], i) => (
                    <AccordionItem value={String(i)} key={title}>
                      <AccordionTrigger>{title}</AccordionTrigger>
                      <AccordionContent>
                        <p>{text}</p>
                      </AccordionContent>
                    </AccordionItem>
                  ))}
              </Accordion>
              {!issues.some((i) =>
                i.join(" ").toLowerCase().includes(query.toLowerCase()),
              ) && (
                <p>
                  No matching answer. Try a shorter search, such as “token” or
                  “agent”.
                </p>
              )}
              <div className="knowledge-note">
                <Terminal size={21} />
                <div>
                  <h3>Check the host first.</h3>
                  <p>
                    <code>mace status</code> tells you whether the server is
                    running. The log location is printed by{" "}
                    <code>mace start</code>. Native agent logs are in Activity.
                  </p>
                </div>
              </div>
            </>
          ) : page === "docs" ? (
            <Docs />
          ) : (
            <>
              <div className="about-illustration">
                <img
                  src="/art/mace-workspace.png"
                  alt="Geometric host and phone connected by a cable"
                />
              </div>
              <div className="about-copy">
                <h2>Work on the machine you already have.</h2>
                <p>
                  Mace puts the CLI agents, files, and apps on your host within
                  reach of a browser. Use your phone, laptop, or another
                  computer to continue working with that host.
                </p>
                <p>
                  A conversation is the main place to work. Groups let agents
                  take turns, share context, and work toward a task. Host
                  controls and activity records are available when you want to
                  look closer.
                </p>
                <Separator />
                <dl>
                  <div>
                    <dt>Name</dt>
                    <dd>MACE</dd>
                  </div>
                  <div>
                    <dt>Full name</dt>
                    <dd>Multi Agent Coordination and Execution</dd>
                  </div>
                  <div>
                    <dt>Version</dt>
                    <dd>0.1.0</dd>
                  </div>
                  <div>
                    <dt>Release</dt>
                    <dd>Working prototype</dd>
                  </div>
                </dl>
                <h3>What this version does</h3>
                <p>
                  Native CLI conversations, groups, stopping controls, file
                  browsing and previews, desktop app launch, direct host
                  connections, and checked file transfers.
                </p>
                <h3>What to expect</h3>
                <p>
                  Live agent behavior depends on the CLI installed on your host.
                  Desktop apps need a graphical host session. Media formats
                  depend on your browser. Mace launches host apps, but does not
                  stream a remote desktop.
                </p>
              </div>
            </>
          )}
        </main>
      )}
      <footer className="site-footer">
        <div className="footer-brand">
          <a href="#home">
            <img src="/mace-mark.svg" alt="" />
            MACE
          </a>
          <p>Multi Agent Coordination and Execution</p>
          <span>Version 0.1.0</span>
        </div>
        <nav aria-label="Footer navigation">
          {sections.map(([id, label]) => (
            <a href={"#" + id} key={id}>
              {label}
            </a>
          ))}
        </nav>
        <button onClick={() => go("messenger")}>
          Open workspace <ArrowUpRight size={17} />
        </button>
      </footer>
    </div>
  );
}
function ActivityIcon() {
  return <BookOpen size={23} />;
}
function Docs() {
  return (
    <div className="docs-body">
      <section>
        <h2>Server commands</h2>
        <div className="reference-table">
          {[
            ["mace start", "Start the server and print your sign-in links."],
            ["mace start --port 0", "Choose an available port automatically."],
            ["mace status", "Check whether the managed server is running."],
            ["mace info", "Print current addresses, sign-in links, and token."],
            ["mace token", "Print the active or saved access token."],
            ["mace doctor", "Check port availability, paths, and installed agents."],
            ["mace logs --lines 50", "Read recent server logs. Add --follow to watch."],
            ["mace agents", "List conversational agent instances."],
            ["mace ps", "List supervised native processes."],
            ["mace restart", "Restart with the saved host settings."],
            ["mace stop", "Stop the server and its active work gracefully."],
            ["mace kill", "Force the managed daemon to stop."],
            ["mace version", "Print the MACE name, full name, and version."],
            ["maced", "Run the daemon in the foreground."],
          ].map(([command, desc]) => (
            <div key={command}>
              <code>{command}</code>
              <p>{desc}</p>
            </div>
          ))}
        </div>
      </section>
      <section>
        <h2>Optional configuration</h2>
        <p>
          You do not need these settings to start. They are environment
          overrides for host administration.
        </p>
        <div className="reference-table">
          {[
            [
              "MACE_HOME",
              "Persistent state location. Defaults to the user’s configuration folder under mace.",
            ],
            ["MACE_HTTP_HOST", "Listener address. Default: 0.0.0.0."],
            ["MACE_HTTP_PORT", "Server port. Default: 8080."],
            [
              "MACE_AUTH_SECRET",
              "Overrides the automatically generated access token.",
            ],
            [
              "MACE_FILE_ROOT",
              "Restricts file browsing and transfers. Default: /.",
            ],
            [
              "MACE_MAX_CONCURRENT_AGENTS",
              "Concurrent native process limit. Default: 10.",
            ],
            ["MACE_TS_AUTHKEY", "Enrolls the embedded node in your Tailnet."],
            ["MACE_TS_HOSTNAME", "Name of the embedded Tailnet host."],
          ].map(([key, desc]) => (
            <div key={key}>
              <code>{key}</code>
              <p>{desc}</p>
            </div>
          ))}
        </div>
      </section>
      <section>
        <h2>Conversation modes</h2>
        <p>
          Reply limit counts total agent replies across the group. Goal oriented
          mode supplies your stated goal and stops on a completion report or the
          configured reply budget. Continuous mode takes turns until paused or
          an error occurs. An agent can reply in only one conversation at a
          time. Runs pause after a host restart.
        </p>
      </section>
      <section>
        <h2>Agent adapters</h2>
        <p>
          Mace discovers Codex, Claude Code, Antigravity, and Muse on PATH. It
          invokes each CLI in its native headless mode, provides recent
          conversation context, and stores the resulting response. Provider
          login, permissions, and availability come from that CLI. There is no
          terminal setup form in the everyday launch flow.
        </p>
      </section>
      <section>
        <h2>Host-to-host protocol</h2>
        <p>
          Connect another host with its address and token. Remote agent IDs
          combine the host and session IDs. Remote turns are authenticated and
          cancel when the conversation is paused. Local agents share filesystem
          paths. Attached remote files are copied with SHA-256 verification,
          with a current limit of 32 MiB per file.
        </p>
      </section>
      <section>
        <h2>API reference</h2>
        <div className="reference-table">
          {[
            ["GET /api/v1/catalog", "Installed agent availability."],
            [
              "GET, POST /api/v1/sessions",
              "List or create conversational agent instances.",
            ],
            [
              "POST /api/v1/sessions/{id}/control",
              "Stop, terminate, resume, or remove an agent.",
            ],
            ["GET, POST /api/v1/rooms", "List or create groups."],
            [
              "GET, POST /api/v1/chats/{room}/messages",
              "Read or send conversation messages.",
            ],
            [
              "GET, POST /api/v1/chats/{room}/policy",
              "Read or set conversation mode.",
            ],
            ["POST /api/v1/chats/{room}/pause", "Cancel an active run."],
            ["GET /api/v1/activity", "Recent audit records."],
            ["GET, POST /api/v1/peers", "List or connect hosts."],
            ["GET, POST /api/v1/transfers", "List or request file transfers."],
          ].map(([key, desc]) => (
            <div key={key}>
              <code>{key}</code>
              <p>{desc}</p>
            </div>
          ))}
        </div>
        <p>
          Protected API requests use the host Bearer token. Browser sign-in
          exchanges the token for an HttpOnly session cookie. Agent IDs are
          routing labels within the single-owner host.
        </p>
      </section>
    </div>
  );
}
