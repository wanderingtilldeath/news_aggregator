import type { Metadata } from "next";
import Link from "next/link";
import MapLibrary from "@/components/map-library";
import index from "@/lib/data/maps/index.json";
import world from "@/lib/data/world.json";
import type { MapSummary } from "@/lib/maps";

export const metadata: Metadata = {
  title: "Maps for revision · Postpaper",
  description: "Ready-made maps for revision: states, rivers, soils and climate, wildlife reserves, ports and more. Study them, mark your own places, or test yourself.",
};

// The library of ready-made maps. Choosing one opens its own page (study it, or take a ten-question test).
export default function Page() {
  return (
    <main className="content np-page" id="view">
      <header className="np-head">
        <div>
          <h1>Maps for revision</h1>
          <p className="np-count">Choose the kind of map you want. Study it, mark your own places with notes, or test yourself: ten places, tap where each one is.</p>
        </div>
      </header>
      <MapLibrary maps={index as MapSummary[]} world={world} />
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
