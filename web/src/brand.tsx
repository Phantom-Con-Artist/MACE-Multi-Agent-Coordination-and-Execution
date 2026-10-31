// The MACE logo, drawn from the same measured geometry as /brand/*.svg. Colors
// come from --logo-tile and --logo-shape, so it follows the palette. With
// "assemble", each tile's triangle slides in from the direction it points,
// one tile after another.

const W = 390,
  H = 600,
  R = 25,
  GAP = 14;
const triangles: Record<string, string> = {
  M: "20,8 370,8 194.5,566",
  A: "194.5,34 20,592 370,592",
  C: "32,299.5 382,20 382,580",
  E: "32,299.5 382,20 382,580",
};
const pills = [
  [182, 176, 149],
  [79, 262, 265],
  [189, 344, 148],
] as const;

function Tile({ letter, x, id }: { letter: string; x: number; id: string }) {
  return (
    <g transform={`translate(${x} 0)`} className={"logo-tile logo-" + letter}>
      <clipPath id={id}>
        <rect width={W} height={H} rx={R} />
      </clipPath>
      <g clipPath={`url(#${id})`}>
        <rect width={W} height={H} className="logo-bg" />
        <g className="logo-shape">
          <polygon
            points={triangles[letter]}
            strokeWidth={22}
            strokeLinejoin="round"
          />
        </g>
        {letter === "E" &&
          pills.map(([px, py, pw], i) => (
            <rect
              key={i}
              className="logo-pill"
              x={px}
              y={py}
              width={pw}
              height={75}
              rx={37.5}
            />
          ))}
      </g>
    </g>
  );
}

let uid = 0;

export function MaceLogo({
  assemble = false,
  inverse = false,
  className = "",
  title = "MACE",
}: {
  assemble?: boolean;
  inverse?: boolean;
  className?: string;
  title?: string;
}) {
  const id = "mace-logo-" + uid++;
  return (
    <svg
      viewBox={`0 0 ${4 * W + 3 * GAP} ${H}`}
      role="img"
      aria-label={title}
      className={
        "mace-logo" +
        (assemble ? " assemble" : "") +
        (inverse ? " inverse" : "") +
        (className ? " " + className : "")
      }
    >
      {["M", "A", "C", "E"].map((letter, i) => (
        <Tile key={letter} letter={letter} x={i * (W + GAP)} id={`${id}-${i}`} />
      ))}
    </svg>
  );
}

/** The E tile alone: the app mark. */
export function MaceMark({
  className = "",
  title = "MACE",
  inverse = false,
}: {
  className?: string;
  title?: string;
  inverse?: boolean;
}) {
  const id = "mace-mark-" + uid++;
  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      role="img"
      aria-label={title}
      className={
        "mace-logo mace-mark" + (inverse ? " inverse" : "") + (className ? " " + className : "")
      }
    >
      <Tile letter="E" x={0} id={id} />
    </svg>
  );
}

/**
 * The working signal: the E tile's three message pills light up in turn.
 * It replaces generic typing dots everywhere an agent is busy.
 */
export function WorkingPills({ label }: { label?: string }) {
  return (
    <span className="working-pills" role={label ? "img" : undefined} aria-label={label} aria-hidden={label ? undefined : true}>
      <i />
      <i />
      <i />
    </span>
  );
}

/** One tile of the mark, used for agent and group avatars. */
export function MaceTile({
  letter,
  inverse = false,
  glyph = false,
}: {
  letter: string;
  inverse?: boolean;
  glyph?: boolean;
}) {
  const id = "mace-tile-" + uid++;
  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      aria-hidden="true"
      preserveAspectRatio="xMidYMid slice"
      className={"mace-logo mace-tile" + (inverse ? " inverse" : "") + (glyph ? " glyph" : "")}
    >
      <Tile letter={letter} x={0} id={id} />
    </svg>
  );
}
