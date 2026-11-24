"use client";

import Link from "next/link";
import { useState, useSyncExternalStore } from "react";
import india from "@/lib/data/india.json";
import { COMBOS, KINDS, countByKind, filterMaps } from "@/lib/map-catalog";
import { SCORES_KEY, parseScores } from "@/lib/map-test";
import type { MapSummary } from "@/lib/maps";
import { readLocal, subscribeLocal } from "@/lib/local";
import MapCard from "./map-card";

// The map library: choose a kind of map (or search), then open one. Each card shows a small picture and, once the reader
// has taken its test, their best score (kept in this browser). The outline every picture sits on is drawn once here.
export default function MapLibrary({ maps, world }: { maps: MapSummary[]; world: { w: number; h: number; d: string } }) {
  const [kind, setKind] = useState("");
  const [query, setQuery] = useState("");
  const scores = parseScores(useSyncExternalStore(subscribeLocal, () => readLocal(SCORES_KEY), () => ""));
  const kinds = countByKind(maps);
  const shown = filterMaps(maps, kind, query);
  const licences = [...new Map(maps.map((m) => [`${m.source}|${m.licence}`, m])).values()];

  return (
    <>
      <svg width="0" height="0" aria-hidden="true" focusable="false" style={{ position: "absolute" }}>
        <defs>
          <g id="mp-india" className="mp-shape">
            {india.states.map((s) => (
              <path key={s.id} d={s.d} />
            ))}
          </g>
          <path id="mp-world" className="mp-shape" d={world.d} />
        </defs>
      </svg>

      <div className="mp-picker">
        <div className="mp-kinds" role="group" aria-label="Kind of map">
          <button type="button" aria-pressed={!kind} onClick={() => setKind("")}>
            All maps <span>{maps.length}</span>
          </button>
          {kinds.map((k) => (
            <button key={k.id} type="button" aria-pressed={kind === k.id} onClick={() => setKind(k.id)} title={k.blurb}>
              {k.label} <span>{k.count}</span>
            </button>
          ))}
        </div>
        <label className="sr-only" htmlFor="mp-q">
          Search the maps
        </label>
        <input id="mp-q" type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search the maps: rivers, ports, tiger…" autoComplete="off" />
      </div>

      <nav className="mp-combo-row" aria-label="Combined maps">
        <span>Combine:</span>
        {COMBOS.map((c) => (
          <Link key={c.label} href={`/maps/combine?${c.ids.map((id) => `ids=${id}`).join("&")}`} title={c.blurb}>
            {c.label}
          </Link>
        ))}
        <Link href="/maps/combine">Pick your own</Link>
        <Link href="/maps/news" className="mp-news-link">
          In the news this week
        </Link>
      </nav>

      {shown.length === 0 ? (
        <p className="empty">No map matches that. Try another kind or a shorter search.</p>
      ) : (
        // One section a subject, each under its own divider heading, in the order of KINDS (History, then Heritage and culture, ...).
        KINDS.map((k) => ({ ...k, maps: shown.filter((m) => m.category === k.id) }))
          .filter((g) => g.maps.length > 0)
          .map((g) => (
            <section key={g.id} className="mp-group" aria-labelledby={`mp-g-${g.id}`}>
              <header className="mp-group-head">
                <h2 id={`mp-g-${g.id}`}>{g.label}</h2>
                <span>
                  {g.maps.length} {g.maps.length === 1 ? "map" : "maps"}
                </span>
                <p>{g.blurb}</p>
              </header>
              <div className="mp-grid">
                {g.maps.map((m) => (
                  <MapCard key={m.id} map={m} best={scores[m.id]} world={world} />
                ))}
              </div>
            </section>
          ))
      )}

      <details className="mp-data">
        <summary>Where the maps come from</summary>
        <p className="fine-print">
          Every map is built from open data and carries its source and licence below; none is copied from a coaching site or a textbook. Outlines are indicative, not an authority on borders, and the India outline is still to be checked against the Survey of
          India one.
        </p>
        <ul>
          {licences.map((m) => (
            <li key={`${m.source}|${m.licence}`}>
              <b>{m.title}</b>: {m.source}. Licence: {m.licence}.
            </li>
          ))}
        </ul>
      </details>
    </>
  );
}
