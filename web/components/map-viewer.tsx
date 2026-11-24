"use client";

import { useState, useSyncExternalStore } from "react";
import type { MapLayer } from "@/lib/maps";
import type { WorldOutline } from "./map-bases";
import MapStudy from "./map-study";
import MapTest from "./map-test";

// One ready-made map with its two ways to use it: Study (browse, mark places, take notes) and Test (ten places to find).
const onHash = (fn: () => void) => {
  window.addEventListener("hashchange", fn);
  return () => window.removeEventListener("hashchange", fn);
};

export default function MapViewer({ map, world }: { map: MapLayer; world?: WorldOutline }) {
  // /maps/tiger#test opens the test; the tabs override it once pressed.
  const hash = useSyncExternalStore(onHash, () => window.location.hash, () => "");
  const [chosen, setChosen] = useState<"study" | "test" | null>(null);
  const mode = chosen ?? (hash === "#test" ? "test" : "study");
  const setMode = setChosen;

  return (
    <>
      <header className="np-head">
        <div>
          <h1>{map.title}</h1>
          <p className="np-count">{map.blurb}</p>
        </div>
        <div className="np-tools" role="tablist" aria-label="How to use this map">
          <button type="button" role="tab" aria-selected={mode === "study"} className={mode === "study" ? "on" : undefined} onClick={() => setMode("study")}>
            Study
          </button>
          <button type="button" role="tab" aria-selected={mode === "test"} className={mode === "test" ? "on" : undefined} onClick={() => setMode("test")}>
            Test yourself
          </button>
        </div>
      </header>
      {mode === "study" ? <MapStudy map={map} world={world} /> : <MapTest map={map} world={world} />}
    </>
  );
}
