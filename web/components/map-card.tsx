import Link from "next/link";
import { kindLabel } from "@/lib/map-catalog";
import type { MapSummary } from "@/lib/maps";
import { project, projectWorld } from "@/lib/maps";
import india from "@/lib/data/india.json";

const noun = (m: MapSummary) => (m.kind === "line" ? "rivers" : m.kind === "states" || m.kind === "region" ? "states" : "places");

// One map in the library: a small picture of it (the outline is the shared #mp-india shape defined once on the page; dots,
// shaded states or river lines go on top), its title, kind and what it teaches, and the reader's best test score.
export default function MapCard({ map, best, world }: { map: MapSummary; best?: { best: number; total: number }; world: { w: number; h: number } }) {
  const onWorld = map.base === "world";
  return (
    <Link className="mp-card" href={`/maps/${map.id}`}>
      <svg viewBox={onWorld ? `0 0 ${world.w} ${world.h}` : `0 0 ${india.w} ${india.h}`} className="mp-thumb" aria-hidden="true" focusable="false">
        {map.kind === "region" ? (
          india.states.map((s) => <path key={s.id} d={s.d} className="mp-shape" style={{ fill: map.shade?.[s.id] }} />)
        ) : (
          <use href={onWorld ? "#mp-world" : "#mp-india"} />
        )}
        {map.paths?.map((d, i) => (
          <path key={i} d={d} className="mp-line" style={{ stroke: map.color }} />
        ))}
        {map.dots?.map(([lon, lat], i) => {
          const p = onWorld ? projectWorld(lon, lat) : project(lon, lat);
          return <circle key={i} cx={p.x} cy={p.y} r={onWorld ? 7 : 15} style={{ fill: map.color }} />;
        })}
      </svg>
      <span className="mp-kind">{kindLabel(map.category)}</span>
      <strong>{map.title}</strong>
      <span className="mp-blurb">{map.blurb}</span>
      <span className="mp-foot">
        {map.count} {noun(map)}
        {best && (
          <b>
            Best {best.best}/{best.total}
          </b>
        )}
      </span>
    </Link>
  );
}
