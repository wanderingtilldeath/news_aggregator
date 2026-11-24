"use client";

import Link from "next/link";
import { useState } from "react";
import type { NewsPlace } from "@/lib/map-news-data";
import { cleanTitle } from "@/lib/view";
import { INDIA_BASE } from "./map-bases";
import MapCanvas, { useMapView } from "./map-canvas";

// This week's stories on the map of India: a state is shaded by how many stories name it, other places (a river, a dam, a
// city, a park) are dots or lines, and the list beside the map gives the stories of the one you pick. Tap a shaded state, a
// dot or a name in the list. All text is drawn as text, never as HTML.
const shade = (n: number) => `color-mix(in srgb, var(--accent) ${Math.min(80, 20 + n * 12)}%, var(--surface-2))`;

export default function MapNews({ places }: { places: NewsPlace[] }) {
  const state = useMapView(INDIA_BASE);
  const [sel, setSel] = useState(places[0]?.key ?? "");
  const picked = places.find((p) => p.key === sel);
  const fills = Object.fromEntries(places.filter((p) => p.shape).map((p) => [p.shape!, shade(p.count)]));
  const byShape = new Map(places.filter((p) => p.shape).map((p) => [p.shape!, p.key]));

  return (
    <div className="map-layout">
      <MapCanvas
        base={INDIA_BASE}
        state={state}
        fills={fills}
        marked={picked?.shape ? { [picked.shape]: "ask" } : undefined}
        label={`Map of India with ${places.length} places in this week's news`}
        onTap={(p) => {
          const k = p.shape ? byShape.get(p.shape) : undefined;
          if (k) setSel(k);
        }}
      >
        {({ dot }) => (
          <>
            {places.filter((p) => p.d).map((p) => (
              <g key={p.key}>
                <path d={p.d} className={`map-river${sel === p.key ? " on" : ""}`} style={{ stroke: "var(--accent)" }} pointerEvents="none" />
                <path d={p.d} className="map-river-hit" onPointerDown={(e) => e.stopPropagation()} onPointerUp={(e) => { e.stopPropagation(); setSel(p.key); }}>
                  <title>{p.name}</title>
                </path>
              </g>
            ))}
            {places.filter((p) => p.x !== undefined && p.y !== undefined).map((p) => (
              <circle
                key={p.key}
                cx={p.x}
                cy={p.y}
                r={dot * (1 + Math.min(p.count, 5) * 0.25)}
                className={`map-dot${sel === p.key ? " on" : ""}`}
                style={{ fill: "var(--accent)" }}
                onPointerDown={(e) => e.stopPropagation()}
                onPointerUp={(e) => {
                  e.stopPropagation();
                  setSel(p.key);
                }}
              >
                <title>{`${p.name}: ${p.count} ${p.count === 1 ? "story" : "stories"}`}</title>
              </circle>
            ))}
          </>
        )}
      </MapCanvas>

      <div className="map-side">
        {picked && (
          <section className="map-card" aria-label={picked.name}>
            <h2>{picked.name}</h2>
            <p className="fine-print">
              {picked.on.join(", ")} · {picked.count} {picked.count === 1 ? "story" : "stories"} this week
            </p>
            <ul className="mp-news-stories">
              {picked.stories.map((s) => (
                <li key={s.id}>
                  <Link href={`/story/${encodeURIComponent(s.id)}`}>{cleanTitle(s.title)}</Link>
                  <span>{s.source}</span>
                </li>
              ))}
            </ul>
          </section>
        )}
        <section className="map-card" aria-label="Places in the news">
          <h2>
            In the news <span className="map-n">{places.length}</span>
          </h2>
          <p className="fine-print">Darker states and bigger dots have more stories. Pick a place to read them.</p>
          <ul className="map-items">
            {places.slice(0, 40).map((p) => (
              <li key={p.key}>
                <div>
                  <strong>{p.name}</strong>
                  <span className="map-meta">{p.shape ? "State" : p.on[0]}</span>
                </div>
                <button type="button" className="btn" aria-pressed={sel === p.key} onClick={() => setSel(p.key)} aria-label={`${p.name}: ${p.count} stories`}>
                  {p.count}
                </button>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}
