import {
  Check,
  FileText,
  Globe,
  MousePointerClick,
  Pencil,
  Search,
  Terminal,
  Wrench,
  X,
} from "lucide-react";

export type Step = {
  id: string;
  kind: string;
  title: string;
  detail?: string;
  state: "running" | "done" | "failed";
  at: string;
};

const icons: Record<string, typeof FileText> = {
  read: FileText,
  edit: Pencil,
  command: Terminal,
  search: Search,
  web: Globe,
  browser: MousePointerClick,
};

// Titles mark file names and commands with backticks, as the CLIs do.
function Title({ text }: { text: string }) {
  return (
    <>
      {text.split("`").map((part, i) =>
        i % 2 ? <code key={i}>{part}</code> : <span key={i}>{part}</span>,
      )}
    </>
  );
}

function StepRow({ step }: { step: Step }) {
  const Icon = icons[step.kind] || Wrench;
  return (
    <li className="step" data-state={step.state} title={step.detail || undefined}>
      <span className="step-icon" aria-hidden="true">
        <Icon size={14} />
      </span>
      <span className="step-title">
        <Title text={step.title} />
      </span>
      <span className="step-state" aria-label={step.state}>
        {step.state === "running" ? (
          <span className="step-spinner" />
        ) : step.state === "failed" ? (
          <X size={13} />
        ) : (
          <Check size={13} />
        )}
      </span>
    </li>
  );
}

/** The steps an agent took for a finished reply, collapsed by default. */
export function StepList({ steps }: { steps: Step[] }) {
  if (!steps.length) return null;
  const failed = steps.filter((s) => s.state === "failed").length;
  return (
    <details className="steps">
      <summary>
        {steps.length === 1 ? "1 step" : `${steps.length} steps`}
        {failed ? ` · ${failed} failed` : ""}
      </summary>
      <ol>
        {steps.map((s) => (
          <StepRow key={s.id} step={s} />
        ))}
      </ol>
    </details>
  );
}

/**
 * A live card for an agent that is working, shaped like its reply bubble so it
 * reads as the agent composing. The newest steps stay visible.
 */
export function WorkCard({
  name,
  steps,
  showName,
}: {
  name: string;
  steps: Step[];
  showName: boolean;
}) {
  const visible = steps.slice(-5);
  const hidden = steps.length - visible.length;
  const current = [...steps].reverse().find((s) => s.state === "running");
  return (
    <article className="bubble incoming work-card" aria-live="polite" aria-busy="true">
      <div className="work-head">
        {showName && <strong className="sender">{name}</strong>}
        <span className="typing" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span className="sr-only">
          {name} is working{current ? `: ${current.title.replaceAll("`", "")}` : ""}
        </span>
      </div>
      {steps.length > 0 && (
        <ol className="live-steps">
          {hidden > 0 && <li className="step-more">{hidden} earlier steps</li>}
          {visible.map((s) => (
            <StepRow key={s.id} step={s} />
          ))}
        </ol>
      )}
    </article>
  );
}
