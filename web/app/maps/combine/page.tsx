import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import MapViewer from "@/components/map-viewer";
import index from "@/lib/data/maps/index.json";
import world from "@/lib/data/world.json";
import { COMBOS, MAX_COMBINED, combinable, combine } from "@/lib/map-catalog";
import { MAPS } from "@/lib/maps-registry";
import type { MapSummary } from "@/lib/maps";

export const metadata: Metadata = {
  title: "Combine maps for revision · Postpaper",
  description: "Draw two to four maps on one outline: rivers with dams, ports with airports, parks with tiger reserves. Study them together or test yourself on all of them.",
};

const href = (ids: readonly string[]) => `/maps/combine?${ids.map((id) => `ids=${encodeURIComponent(id)}`).join("&")}`;

// The maps named in ?ids=a&ids=b (or ?ids=a,b), only ones that exist, each once.
const chosen = (v: string | string[] | undefined) => [...new Set((Array.isArray(v) ? v : [v ?? ""]).flatMap((s) => s.split(",")).filter((id) => id in MAPS))];

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const ids = chosen((await searchParams).ids);
  const result = ids.length ? combine(ids.map((id) => MAPS[id])) : null;
  const maps = (index as MapSummary[]).filter(combinable);

  return (
    <>
      <details className="mp-combine-box" open={!result?.map}>
      <summary>{result?.map ? "Change the maps" : "Choose the maps"}</summary>
      <form method="get" className="mp-combine" aria-label="Choose the maps to combine">
        {(["india", "world"] as const).map((base) => (
          <fieldset key={base}>
            <legend>{base === "india" ? "India" : "The world"}</legend>
            <div>
              {maps
                .filter((m) => m.base === base)
                .map((m) => (
                  <label key={m.id}>
                    <input type="checkbox" name="ids" value={m.id} defaultChecked={ids.includes(m.id)} /> {m.title}
                  </label>
                ))}
            </div>
          </fieldset>
        ))}
        <div className="mp-combine-go">
          <button type="submit" className="btn primary">
            Show them together
          </button>
          <span className="fine-print">Two to {MAX_COMBINED} maps, all of India or all of the world.</span>
        </div>
      </form>
      </details>

      {result?.error && <p className="note err" role="alert">{result.error}</p>}
      {result?.map ? (
        <MapViewer key={result.map.id} map={result.map} world={result.map.base === "world" ? world : undefined} />
      ) : (
        !result && (
          <>
            <p className="fine-print">Or start from one of these:</p>
            <ul className="mp-combos">
              {COMBOS.map((c) => (
                <li key={c.label}>
                  <Link href={href(c.ids)}>
                    <strong>{c.label}</strong>
                    <span>{c.blurb}</span>
                  </Link>
                </li>
              ))}
            </ul>
          </>
        )
      )}
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/maps/combine">) {
  return (
    <main className="content np-page" id="view">
      <nav className="crumbs" aria-label="Breadcrumb">
        <Link href="/maps">Maps</Link>
        <span aria-hidden="true">/</span>
        <span>Combine</span>
      </nav>
      <header className="np-head">
        <div>
          <h1>Combine maps</h1>
          <p className="np-count">Put two to four maps on one outline, then study them together or test yourself on every place at once.</p>
        </div>
      </header>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
