"use client";

import { useState, useSyncExternalStore } from "react";
import { labellable } from "@/lib/map-print";
import { erasOf, forEra, type MapLayer } from "@/lib/maps";
import { baseFor, type WorldOutline } from "./map-bases";
import MapPosterView from "./map-poster-view";
import MapStudy from "./map-study";
import MapTest from "./map-test";

// One ready-made map with its ways to use it: Study (browse, mark places, take notes), Test (ten places to find) and, for dot
// maps on India, Labelled (the poster with a line from every dot to its name).
const onHash = (fn: () => void) => {
  window.addEventListener("hashchange", fn);
  return () => window.removeEventListener("hashchange", fn);
};

export default function MapViewer({ map, world }: { map: MapLayer; world?: WorldOutline }) {
  // /maps/tiger#test opens the test; the tabs override it once pressed.
  const hash = useSyncExternalStore(onHash, () => window.location.hash, () => "");
  const [chosen, setChosen] = useState<"study" | "test" | "labelled" | null>(null);
  const canLabel = labellable(map);
  const mode = chosen ?? (hash === "#test" ? "test" : hash === "#labelled" && canLabel ? "labelled" : "study");
  const setMode = setChosen;
  // A history map told in periods offers them as chips; Study and Test then see only that period's places.
  const [era, setEra] = useState("");
  const eras = erasOf(map);
  const shown = forEra(map, era);

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
          {canLabel && (
            <button type="button" role="tab" aria-selected={mode === "labelled"} className={mode === "labelled" ? "on" : undefined} onClick={() => setMode("labelled")}>
              Labelled
            </button>
          )}
        </div>
      </header>
      {eras.length > 0 && (
        <div className="mp-kinds mp-eras" role="group" aria-label="Period">
          <button type="button" aria-pressed={!era} onClick={() => setEra("")}>
            All periods <span>{map.items?.length}</span>
          </button>
          {eras.map((e) => (
            <button key={e} type="button" aria-pressed={era === e} onClick={() => setEra(e)}>
              {e} <span>{forEra(map, e).items?.length}</span>
            </button>
          ))}
        </div>
      )}
      {mode === "study" && <MapStudy key={era} map={shown} world={world} />}
      {mode === "test" && <MapTest key={era} map={shown} world={world} />}
      {mode === "labelled" && <MapPosterView key={era} map={shown} base={baseFor(shown, world)} />}
    </>
  );
}
