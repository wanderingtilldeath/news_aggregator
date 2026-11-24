"use client";

import { useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { MapLayer } from "@/lib/maps";
import type { Base } from "./map-bases";
import { MapPoster, savePicture } from "./map-poster";
import MapSheet from "./map-sheet";
import { useMarks } from "./use-marks";
import { usePrintSheet } from "./use-print-sheet";

// The Labelled tab: the poster, with Print and Download picture.
export default function MapPosterView({ map, base }: { map: MapLayer; base: Base }) {
  const svg = useRef<SVGSVGElement>(null);
  const [printing, print] = usePrintSheet();
  const [failed, setFailed] = useState(false);
  const [marks] = useMarks();

  return (
    <div className="mp-poster">
      <div className="map-row">
        <button type="button" className="btn" onClick={print}>
          Print the labelled map
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => {
            setFailed(false);
            if (svg.current) savePicture(svg.current, `${map.id.replace(/[^a-z0-9+-]+/gi, "-")}-labelled.png`).catch(() => setFailed(true));
          }}
        >
          Download picture
        </button>
      </div>
      {failed && <p className="note err" role="alert">This browser could not make the picture. Use Print and choose Save as PDF.</p>}
      <MapPoster map={map} base={base} marks={marks} svgRef={svg} />
      <p className="fine-print">Dots are where the places are; the lines lead to their names. Shaded states hold at least one; places ringed in red are the ones you starred, and your notes are listed under the picture. Positions are approximate.</p>
      {printing && createPortal(<MapSheet map={map} base={base} mode="poster" marks={marks} />, document.body)}
    </div>
  );
}
