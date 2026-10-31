import { useEffect, useRef, useState } from "react";
import {
  ArrowRight,
  Copy,
  Check,
  Menu,
  X,
  Sun,
  Moon,
  ArrowUpRight,
  Terminal,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Accordion,
  AccordionItem,
  AccordionTrigger,
  AccordionContent,
} from "@/components/ui/accordion";
import { Separator } from "@/components/ui/separator";
import "./public-site.css";
import "./landing.css";
import Landing from "./landing";
import { MaceLogo, MaceMark } from "./brand";
import { HyperbolicDisk, StepArt } from "./art";

const sections = [
  ["home", "Home"],
  ["manual", "Manual"],
  ["docs", "Docs"],
  ["troubleshoot", "Troubleshoot"],
  ["about", "About"],
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
    "Tap +, choose New group, name it, and pick agents. When you write without mentioning anyone, every agent answers in parallel. Write @handle to address one agent; agents mention each other the same way. Each agent keeps one memory across its private chat and its groups. Add or remove members from the conversation menu.",
  ],
  [
    "Set a stopping point",
    "Reply limit caps how many replies agents give to each of your messages. Goal oriented mode asks agents to work toward your goal, then stops on their completion report or the limit. Continuous mode keeps the team talking until you pause, a reply fails, or everyone passes. A completion report is a claim to review, not a guarantee.",
  ],
  [
    "Pause, stop, or remove an agent",
    "Pause, in the conversation header while agents work, stops that conversation. Pause everything stops every agent on every connected computer. Stop cancels one agent's work. Terminate makes it unavailable until resumed. Remove takes it out of your chats and groups while keeping history. You can write at any time, even while agents work.",
  ],
  [
    "Open files and apps",
    "Host files lets you browse folders, read text, preview pictures, and play supported audio or video. When an agent links a file in its reply, tap it to open it. Open an app launches an installed application on the host desktop; its window is not yet streamed to your browser. Activity shows launch results and process output.",
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
    "Use Pause in the conversation header, or Pause everything in the banner. Continuous mode runs until paused. Outside continuous mode, a chain of agents handing work to each other stops after eight hand-offs, and agents that repeat each other are paused automatically.",
  ],
];
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
        <a className="site-brand" href="#home" aria-label="MACE home">
          <MaceLogo assemble title="MACE" />
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
        <Landing go={go} />      ) : (
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
                <StepArt step={0} />
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
                <HyperbolicDisk label="A hyperbolic tiling of red and black triangles" />
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
            <MaceMark title="" className="footer-mark" />
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
