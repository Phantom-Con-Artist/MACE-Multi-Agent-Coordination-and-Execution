// Original vector artwork for the public pages, built from the MACE mark.
// The centerpiece is the (2,3,7) triangle tiling of the hyperbolic plane in
// the Poincare disk (scripts/hyperbolic-art.py): congruent triangles that
// shrink toward the rim, in the logo's two alternating colorways.
import { MaceLogo, WorkingPills } from "./brand";
import { Avatar } from "./messenger-parts";

export function HyperbolicDisk({
  className = "",
  reverse = false,
  label,
}: {
  className?: string;
  reverse?: boolean;
  label?: string;
}) {
  return (
    <svg
      viewBox="-280 -280 560 560"
      className={"hyperbolic " + className}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
    >
      <circle r="280" className="hd-rim" />
      <g className={"hd-spin" + (reverse ? " reverse" : "")}>
        <use href="/art/hyperbolic.svg#even" className="hd-even" />
        <use href="/art/hyperbolic.svg#odd" className="hd-odd" />
      </g>
    </svg>
  );
}

/** Step 1: one command on the host; the mark assembles as Mace starts. */
function StartArt() {
  return (
    <div className="step-art start-art" aria-hidden="true">
      <div className="term">
        <div className="term-bar">
          <i />
          <i />
          <i />
        </div>
        <code>
          <span className="prompt">$</span> mace start
        </code>
        <MaceLogo assemble className="term-logo" title="" />
        <code className="term-out">Ready. Open the sign-in link.</code>
      </div>
    </div>
  );
}

/** Step 2: the phone opens the link; a message pill travels to the host. */
function LinkArt() {
  return (
    <div className="step-art link-art" aria-hidden="true">
      <svg viewBox="0 0 400 260">
        <path id="route" className="route" d="M110 170 C 170 60, 250 60, 300 120" />
        <g className="traveler">
          <rect x="-16" y="-6" width="32" height="12" rx="6" />
        </g>
        <rect className="device" x="44" y="96" width="76" height="140" rx="16" />
        <rect className="device-screen" x="52" y="112" width="60" height="100" rx="8" />
        <rect className="chip" x="62" y="128" width="40" height="10" rx="5" />
        <rect className="chip accent" x="70" y="146" width="32" height="10" rx="5" />
        <rect className="chip" x="62" y="164" width="28" height="10" rx="5" />
        <rect className="device" x="276" y="70" width="104" height="80" rx="12" />
        <rect className="device-screen" x="284" y="78" width="88" height="56" rx="6" />
        <rect className="stand" x="316" y="150" width="24" height="18" />
        <rect className="device" x="296" y="166" width="64" height="8" rx="4" />
        <polygon className="chip accent" points="312,92 344,92 328,122" />
      </svg>
    </div>
  );
}

/** Step 3: a group where everyone answers and the work shows as it happens. */
function ChatArt() {
  return (
    <div className="step-art chat-art" aria-hidden="true">
      <div className="mini-chat">
        <div className="mini out">Both of you, check the plan</div>
        <div className="mini in">
          <Avatar id="art-muse" provider="muse" size={26} />
          <span>
            <b>Muse</b>
            The plan ships group chat by Friday.
          </span>
        </div>
        <div className="mini in working">
          <Avatar id="art-agy" provider="agy" size={26} />
          <span>
            <b>
              Antigravity <WorkingPills />
            </b>
            Reading <code>PLAN.md</code>
          </span>
        </div>
      </div>
    </div>
  );
}

export function StepArt({ step }: { step: number }) {
  return step === 0 ? <StartArt /> : step === 1 ? <LinkArt /> : <ChatArt />;
}

/** Two hosts, one conversation: two disks turning in opposite directions. */
export function TwoHosts() {
  return (
    <div className="two-hosts-art" role="img" aria-label="Two computers sharing one conversation">
      <HyperbolicDisk className="host-a" />
      <svg className="link" viewBox="0 0 120 40" aria-hidden="true">
        <path d="M0 20 H120" />
        <rect className="link-pill" x="-14" y="14" width="28" height="12" rx="6" />
      </svg>
      <HyperbolicDisk className="host-b" reverse />
    </div>
  );
}
