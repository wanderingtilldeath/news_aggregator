import Link from "next/link";
import type { Graph } from "@/lib/api";
import { entityHref, type Placed } from "@/lib/graphlayout";

// The knowledge graph as plain SVG: a circle for each entity (bigger for more stories), a line for
// each pair named in the same stories (thicker for more), every circle a link to its page. It is
// drawn on the server from numbers, so it needs no script and no library.
export default function GraphSvg({ size, placed, links, centre, label }: { size: number; placed: Placed[]; links: { a: Placed; b: Placed; count: number }[]; centre?: Placed; label: string }) {
  const max = Math.max(...links.map((l) => l.count), 1);
  return (
    <svg className="graph-svg" viewBox={`0 0 ${size} ${size}`} role="img" aria-label={label}>
      {links.map((l, i) => (
        <line key={i} x1={l.a.x} y1={l.a.y} x2={l.b.x} y2={l.b.y} stroke="var(--line-strong)" strokeWidth={1 + (3 * l.count) / max} opacity={0.35 + (0.5 * l.count) / max} />
      ))}
      {[...(centre ? [centre] : []), ...placed].map((p) => (
        <Link key={p.name} href={entityHref(p.name)}>
          <g className={p === centre ? "gnode centre" : "gnode"}>
            <title>{`${p.name}: ${p.count} ${p.count === 1 ? "story" : "stories"}`}</title>
            <circle cx={p.x} cy={p.y} r={p.r} />
            <text x={p.x} y={p.y + p.r + 13} textAnchor="middle">
              {p.name.length > 18 ? p.name.slice(0, 17) + "…" : p.name}
            </text>
          </g>
        </Link>
      ))}
    </svg>
  );
}

export type { Graph };
