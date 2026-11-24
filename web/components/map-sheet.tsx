import { printPoints } from "@/lib/map-print";
import type { MapLayer } from "@/lib/maps";
import type { Base } from "./map-bases";

// The page that prints when a reader asks for a blank practice map: the outline with a number on every place and lines to
// write the names on, then the key on a second page. It is not on screen (.mp-sheet is hidden outside print) and is drawn
// in plain black, grey and one red so it prints well on any printer. Names are text, never HTML.
export default function MapSheet({ map, base }: { map: MapLayer; base: Base }) {
  const points = printPoints(map, base);
  // Numbers shrink with the picture's width, and a crowded map (a country each) gets smaller ones.
  const size = (base.w / 100) * (points.length > 80 ? 0.8 : 1.1);
  const dots = map.kind === "point";

  return (
    <div className="mp-sheet">
      <h1>{map.title}: blank practice map</h1>
      <p>Write the name of each numbered place on its line. The key is on the next page.</p>
      <svg viewBox={`0 0 ${base.w} ${base.h}`} role="img" aria-label={`Blank ${base.name} outline with ${points.length} numbered places`}>
        {base.shapes.map((s) => (
          <path key={s.id} d={s.d} fill="#f3f0eb" stroke="#888" strokeWidth={0.6} />
        ))}
        {map.lines?.map((l) => (
          <path key={l.id} d={l.d} fill="none" stroke="#555" strokeWidth={1.4} />
        ))}
        {points.map((p) => (
          <g key={p.n}>
            {(dots || map.kind === "line") && <circle cx={p.x} cy={p.y} r={size * 0.45} fill="#b3261e" />}
            <text x={p.x + (dots || map.kind === "line" ? size * 0.7 : 0)} y={p.y + size * 0.35} fontSize={size} textAnchor={dots || map.kind === "line" ? "start" : "middle"} fill="#000" stroke="#fff" strokeWidth={size * 0.25} paintOrder="stroke">
              {p.n}
            </text>
          </g>
        ))}
      </svg>
      <ol className="mp-write">
        {points.map((p) => (
          <li key={p.n} value={p.n}>
            <span />
          </li>
        ))}
      </ol>
      <section className="mp-key">
        <h2>Key: {map.title}</h2>
        <ol>
          {points.map((p) => (
            <li key={p.n} value={p.n}>
              <strong>{p.name}</strong>
              {p.detail ? ` · ${p.detail}` : ""}
            </li>
          ))}
        </ol>
        <p>
          Source: {map.source}. Licence: {map.licence}. {map.approx ? "Approximate. " : ""}From postpaper.in/maps/{map.id}
        </p>
      </section>
    </div>
  );
}
