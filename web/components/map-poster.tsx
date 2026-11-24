"use client";

import type { RefObject } from "react";
import { kindLabel } from "@/lib/map-catalog";
import { LAYOUT, chooseLabelled, layoutCallouts, shadedStates, wrapLabel, type Frame } from "@/lib/map-labels";
import type { Mark } from "@/lib/map-marks";
import { printPoints } from "@/lib/map-print";
import { coverageLine, type MapLayer } from "@/lib/maps";
import type { Base } from "./map-bases";

// The labelled map as a poster: the outline with the states that hold a place shaded, a dot on every place, a line from each
// dot out to its name in the margin, a title box and a legend. It is one SVG drawn from attributes only (no style sheet, no
// theme colours), so it prints the same on any page and can be saved as a picture. With `blank`, an empty box stands in for
// each name, for a student to write in.
const MARGIN = LAYOUT.margin;
const HEAD = 130;
const FONT = "system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif";
const FRAME = LAYOUT;

export function MapPoster({ map, base, blank = false, marks = [], svgRef }: { map: MapLayer; base: Base; blank?: boolean; marks?: Mark[]; svgRef?: RefObject<SVGSVGElement | null> }) {
  const color = (type?: string) => map.types?.find((t) => t.id === type)?.color ?? map.color ?? "#2f7d5b";
  const points = printPoints(map, base);
  const { labelled, keyed } = chooseLabelled(points, map.kind);
  const frame: Frame = { top: FRAME.top, pad: FRAME.pad, maxChars: FRAME.maxChars, lineH: FRAME.lineH, gap: FRAME.gap, w: base.w, bottom: base.h - 20 };
  const callouts = layoutCallouts(labelled.map((p) => ({ id: String(p.n), name: p.name, x: p.x, y: p.y })), frame);
  const shaded = new Set(map.kind === "states" ? [] : shadedStates(map.items, base.shapes));
  const byId = new Map(labelled.map((p) => [String(p.n), p]));
  // The reader's starred places are ringed, and their notes listed under the picture (not on a blank sheet, which is for writing).
  const stars = new Set(blank ? [] : marks.filter((m) => m.map === map.id && m.item).map((m) => m.item));
  const notes = blank ? [] : marks.filter((m) => m.note).slice(0, 8);

  // The key under the picture (places past the first 30), three columns; then the legend and the source line.
  const rows = Math.ceil(keyed.length / 3);
  const keyTop = base.h + 30;
  const notesTop = keyTop + (rows ? rows * 22 + 36 : 0);
  const legendTop = notesTop + (notes.length ? 34 + notes.length * 22 : 0);
  const legend = map.types?.length ? map.types.map((t) => ({ name: t.name, color: t.color })) : [{ name: `${map.title} (${points.length})`, color: color() }];
  const legendH = 28 + legend.length * 28;
  const width = base.w + 2 * MARGIN;
  const height = HEAD + legendTop + Math.max(legendH, 110) + 40;
  const title = wrapLabel(map.title, 22);
  const boxW = 400;
  const boxH = 18 + title.length * 32 + 26;
  const boxX = base.w + MARGIN - boxW - 10;
  const right = base.w + MARGIN - 10;

  return (
    <svg ref={svgRef} className="mp-poster-svg" viewBox={`${-MARGIN} ${-HEAD} ${width} ${height}`} role="img" aria-label={`${map.title}: ${base.name} with ${points.length} places${blank ? ", names left blank" : " and their names"}`} fontFamily={FONT}>
      <rect x={-MARGIN} y={-HEAD} width={width} height={height} fill="#fbf8ee" />
      {base.shapes.map((s) => (
        <path key={s.id} d={s.d} fill={shaded.has(s.id) ? color() : "#f1edd8"} fillOpacity={shaded.has(s.id) ? 0.38 : 1} stroke="#8a8a7a" strokeWidth={0.8} />
      ))}

      {callouts.map((c) => {
        const p = byId.get(c.spot.id)!;
        const left = c.side === "left";
        return (
          <g key={c.spot.id}>
            <polyline points={c.path.map((q) => q.join(",")).join(" ")} fill="none" stroke="#444" strokeWidth={1.2} />
            {p.id && stars.has(p.id) && <circle cx={c.spot.x} cy={c.spot.y} r={13} fill="none" stroke="#b3261e" strokeWidth={2.6} />}
            <circle cx={c.spot.x} cy={c.spot.y} r={7} fill={color(p.type)} stroke="#fff" strokeWidth={1.6} />
            {blank ? (
              <g>
                <rect x={left ? c.labelX - 276 : c.labelX} y={c.ly - 15} width={276} height={30} fill="#fff" stroke="#444" strokeWidth={1} />
                <text x={left ? c.labelX - 270 : c.labelX + 5} y={c.ly - 3} fontSize={12} fill="#666">
                  {p.n}
                </text>
              </g>
            ) : (
              <text x={c.labelX} y={c.ly - ((c.lines.length - 1) * FRAME.lineH) / 2 + FRAME.font / 3} fontSize={FRAME.font} fontWeight={500} fill="#111" textAnchor={left ? "end" : "start"}>
                {c.lines.map((l, i) => (
                  <tspan key={i} x={c.labelX} dy={i ? FRAME.lineH : 0}>
                    {l}
                  </tspan>
                ))}
              </text>
            )}
          </g>
        );
      })}

      {keyed.map((p, i) => (
        <g key={p.n}>
          {p.id && stars.has(p.id) && <circle cx={p.x} cy={p.y} r={10} fill="none" stroke="#b3261e" strokeWidth={2.2} />}
          <circle cx={p.x} cy={p.y} r={5} fill={color(p.type)} stroke="#fff" strokeWidth={1.2} />
          <text x={p.x + 7} y={p.y - 6} fontSize={13} fontWeight={700} fill="#111" stroke="#fbf8ee" strokeWidth={3} paintOrder="stroke">
            {i + 1}
          </text>
        </g>
      ))}
      {keyed.map((p, i) => (
        <text key={`k${p.n}`} x={-MARGIN + 20 + (i % 3) * ((width - 40) / 3)} y={keyTop + 16 + Math.floor(i / 3) * 22} fontSize={15} fill="#111">
          {i + 1}. {p.name}
          {p.detail ? ` (${p.detail.split(" · ")[0]})` : ""}
        </text>
      ))}

      {notes.length > 0 && (
        <g>
          <text x={-MARGIN + 20} y={notesTop + 6} fontSize={16} fontWeight={700} fill="#b3261e">
            My notes
          </text>
          {notes.map((m, i) => (
            <text key={m.id} x={-MARGIN + 20} y={notesTop + 32 + i * 22} fontSize={14} fill="#111">
              {`${m.name}: ${m.note.length > 120 ? m.note.slice(0, 119) + "…" : m.note}`}
            </text>
          ))}
        </g>
      )}

      <rect x={boxX} y={-HEAD + 20} width={boxW} height={boxH} fill="#fff" stroke="#222" strokeWidth={1.5} />
      {title.map((l, i) => (
        <text key={i} x={boxX + boxW / 2} y={-HEAD + 20 + 38 + i * 32} fontSize={30} fontWeight={700} fill="#111" textAnchor="middle">
          {l}
        </text>
      ))}
      <text x={boxX + boxW / 2} y={-HEAD + 20 + boxH - 12} fontSize={18} fill="#444" textAnchor="middle">
        {kindLabel(map.category)}
      </text>

      <rect x={right - 400} y={legendTop} width={400} height={legendH} fill="#fff" stroke="#888" />
      {legend.map((l, i) => (
        <g key={l.name}>
          <circle cx={right - 400 + 26} cy={legendTop + 28 + i * 28} r={8} fill={l.color} stroke="#fff" strokeWidth={1.4} />
          <text x={right - 400 + 44} y={legendTop + 34 + i * 28} fontSize={17} fill="#111">
            {l.name.length > 34 ? l.name.slice(0, 33) + "…" : l.name}
          </text>
        </g>
      ))}
      <text x={-MARGIN + 20} y={legendTop + 22} fontSize={15} fill="#444">
        Representative map. Not to scale.{map.coverage ? ` ${coverageLine(map.coverage, points.length)}.` : ""}
      </text>
      <text x={-MARGIN + 20} y={legendTop + 46} fontSize={13} fill="#555">
        {`Source: ${map.source.length > 130 ? map.source.slice(0, 129) + "…" : map.source}.`}
      </text>
      <text x={-MARGIN + 20} y={legendTop + 68} fontSize={13} fill="#555">
        {`Licence: ${map.licence.length > 130 ? map.licence.slice(0, 129) + "…" : map.licence}.`}
      </text>
      <text x={-MARGIN + 20} y={legendTop + 90} fontSize={13} fill="#555">
        postpaper.in/maps/{map.id}
      </text>
    </svg>
  );
}

// The SVG as a PNG file at twice its size, made in the browser with a canvas (no library).
export async function savePicture(svg: SVGSVGElement, filename: string): Promise<number> {
  const vb = svg.viewBox.baseVal;
  const copy = svg.cloneNode(true) as SVGSVGElement;
  copy.setAttribute("width", String(vb.width));
  copy.setAttribute("height", String(vb.height));
  const url = URL.createObjectURL(new Blob([new XMLSerializer().serializeToString(copy)], { type: "image/svg+xml;charset=utf-8" }));
  try {
    const img = new Image();
    img.src = url;
    await img.decode();
    const canvas = document.createElement("canvas");
    canvas.width = vb.width * 2;
    canvas.height = vb.height * 2;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("no canvas");
    ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
    const blob = await new Promise<Blob | null>((done) => canvas.toBlob(done, "image/png"));
    if (!blob) throw new Error("no picture");
    const a = document.createElement("a");
    a.href = URL.createObjectURL(blob);
    a.download = filename;
    a.click();
    setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
    return blob.size;
  } finally {
    URL.revokeObjectURL(url);
  }
}
