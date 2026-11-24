"use client";

import { useRef, useState, type PointerEvent, type ReactNode } from "react";
import type { Base } from "./map-bases";

const MAX_ZOOM = 10;
const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));

export type View = { z: number; cx: number; cy: number }; // zoom, and the map point at the centre of the picture

// The pan and zoom of one map: where it looks, and the ways to move it. The page holds it, so a button outside the
// canvas (Show on the map, a new question) can move the view too.
export function useMapView(base: Base) {
  const full: View = { z: 1, cx: base.w / 2, cy: base.h / 2 };
  const [view, setView] = useState<View>(full);
  // Keep the picture inside the map when zooming or panning.
  const fit = (z: number, cx: number, cy: number): View => {
    const zz = clamp(z, 1, MAX_ZOOM);
    return { z: zz, cx: clamp(cx, base.w / zz / 2, base.w - base.w / zz / 2), cy: clamp(cy, base.h / zz / 2, base.h - base.h / zz / 2) };
  };
  return {
    view,
    fit,
    set: (v: View) => setView(v),
    zoomBy: (f: number) => setView((v) => fit(v.z * f, v.cx, v.cy)),
    reset: () => setView(full),
    // Centre on a place at a zoom (at least the current one when keep is set).
    focus: (lon: number, lat: number, z: number, keep = false) => {
      const p = base.project(lon, lat);
      setView((v) => fit(keep ? Math.max(z, v.z) : z, p.x, p.y));
    },
  };
}

export type MapViewState = ReturnType<typeof useMapView>;

// The map picture: an outline with shapes you can tap, pan by dragging, zoom with the wheel, two fingers or the buttons.
// What goes on the map (dots, pins, lines) is drawn by `children`, which gets a size for a dot that shrinks as the
// reader zooms in. onTap gets the place tapped and, when it was on a shape, that shape's id. Text is drawn as text,
// never as HTML.
export default function MapCanvas({
  base,
  state,
  label,
  fills,
  marked,
  hideNames,
  placing,
  onTap,
  children,
}: {
  base: Base;
  state: MapViewState;
  label: string;
  fills?: Record<string, string | undefined>;
  marked?: Record<string, "right" | "wrong" | "ask">;
  hideNames?: boolean;
  // Study mode: true while a note is being placed (a crosshair), false otherwise (a hand: a tap only reads the map). Left out, the stylesheet decides (Test).
  placing?: boolean;
  onTap?: (p: { lon: number; lat: number; shape?: string }) => void;
  children?: (ctx: { dot: number }) => ReactNode;
}) {
  const { view, fit, set, zoomBy, reset } = state;
  const svg = useRef<SVGSVGElement>(null);
  const drag = useRef<{ x: number; y: number; cx: number; cy: number; moved: boolean } | null>(null);
  const pointers = useRef(new Map<number, { x: number; y: number }>());
  const pinch = useRef<{ d: number; z: number } | null>(null);

  const vw = base.w / view.z;
  const vh = base.h / view.z;
  const box = `${view.cx - vw / 2} ${view.cy - vh / 2} ${vw} ${vh}`;
  const spread = () => {
    const [a, b] = [...pointers.current.values()];
    return Math.hypot(a.x - b.x, a.y - b.y);
  };

  // The map point under a screen point.
  const at = (clientX: number, clientY: number) => {
    const el = svg.current;
    const inv = el?.getScreenCTM()?.inverse();
    if (!el || !inv) return null;
    const p = new DOMPoint(clientX, clientY).matrixTransform(inv);
    return { x: p.x, y: p.y };
  };

  const down = (e: PointerEvent<SVGSVGElement>) => {
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    e.currentTarget.setPointerCapture(e.pointerId);
    if (pointers.current.size === 2) {
      // A second finger turns the drag into a pinch.
      pinch.current = { d: spread() || 1, z: view.z };
      drag.current = null;
    } else {
      drag.current = { x: e.clientX, y: e.clientY, cx: view.cx, cy: view.cy, moved: false };
    }
  };
  const move = (e: PointerEvent<SVGSVGElement>) => {
    if (pointers.current.has(e.pointerId)) pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY });
    if (pointers.current.size === 2 && pinch.current) {
      const p = pinch.current;
      set(fit(p.z * (spread() / p.d), view.cx, view.cy));
      return;
    }
    const d = drag.current;
    const el = svg.current;
    if (!d || !el) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (!d.moved && Math.hypot(dx, dy) < 5) return;
    d.moved = true;
    const rect = el.getBoundingClientRect();
    // One screen pixel is vw / width map units (the picture is letterboxed, so use the smaller scale).
    const unit = Math.max(vw / rect.width, vh / rect.height);
    set(fit(view.z, d.cx - dx * unit, d.cy - dy * unit));
  };
  const up = (e: PointerEvent<SVGSVGElement>) => {
    pointers.current.delete(e.pointerId);
    if (pointers.current.size < 2) pinch.current = null;
    const d = drag.current;
    drag.current = null;
    if (!d || d.moved || !onTap) return;
    const p = at(e.clientX, e.clientY);
    if (!p) return;
    const { lon, lat } = base.unproject(p.x, p.y);
    if (!base.contains(lon, lat)) return;
    const shape = (document.elementFromPoint(e.clientX, e.clientY) as Element | null)?.getAttribute("data-shape") ?? undefined;
    onTap({ lon, lat, shape });
  };

  return (
    <div className="map-stage">
      <svg
        ref={svg}
        viewBox={box}
        // The world is wide: a frame as tall as India's would leave it floating in empty space.
        style={{ ...(base.id === "world" ? { height: "auto", aspectRatio: "1.9" } : {}), ...(placing === undefined ? {} : { cursor: placing ? "crosshair" : "grab" }) }}
        role="img"
        aria-label={label}
        onPointerDown={down}
        onPointerMove={move}
        onPointerUp={up}
        onPointerCancel={() => {
          pointers.current.clear();
          pinch.current = null;
          drag.current = null;
        }}
        onWheel={(e) => zoomBy(e.deltaY < 0 ? 1.25 : 0.8)}
      >
        {base.shapes.map((s) => (
          <path key={s.id} d={s.d} data-shape={s.id} className={`map-state${marked?.[s.id] ? ` ${marked[s.id]}` : ""}`} style={fills?.[s.id] ? { fill: fills[s.id] } : undefined}>
            {!hideNames && <title>{s.name}</title>}
          </path>
        ))}
        {children?.({ dot: 5 / Math.sqrt(view.z) })}
      </svg>
      <div className="map-zoom" role="group" aria-label="Zoom">
        <button type="button" onClick={() => zoomBy(1.6)} aria-label="Zoom in">
          +
        </button>
        <button type="button" onClick={() => zoomBy(1 / 1.6)} aria-label="Zoom out">
          −
        </button>
        <button type="button" onClick={reset} aria-label="Show the whole map">
          ⌂
        </button>
      </div>
    </div>
  );
}
