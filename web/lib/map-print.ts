import type { Base } from "@/components/map-bases";
import type { MapLayer } from "./maps";

// The blank practice sheet of a map (a printable outline with a number on every place, and a key on a second page).
// This file only works out where each number goes; components/map-sheet.tsx draws it.

export type PrintPoint = { n: number; x: number; y: number; name: string; detail: string; type?: string; id?: string };

const numbers = (d: string) => (d.match(/-?\d+(?:\.\d+)?/g) ?? []).map(Number);

// The middle of the biggest piece of a shape (so the number of a state with islands sits on the mainland, not in the sea).
// The paths in lib/data are absolute M, L and Z only, so every pair of numbers is a point.
export function centre(d: string): { x: number; y: number } | null {
  let best: { x: number; y: number; area: number } | null = null;
  for (const piece of d.split("M").slice(1)) {
    const n = numbers(piece);
    const xs = n.filter((_, i) => i % 2 === 0);
    const ys = n.filter((_, i) => i % 2 === 1);
    if (!xs.length || xs.length !== ys.length) continue;
    const w = Math.max(...xs) - Math.min(...xs);
    const h = Math.max(...ys) - Math.min(...ys);
    if (!best || w * h > best.area) best = { x: (Math.min(...xs) + Math.max(...xs)) / 2, y: (Math.min(...ys) + Math.max(...ys)) / 2, area: w * h };
  }
  return best && { x: best.x, y: best.y };
}

// The first point of a line: where its number is written.
export function startOf(d: string): { x: number; y: number } | null {
  const [x, y] = numbers(d);
  return Number.isFinite(x) && Number.isFinite(y) ? { x, y } : null;
}

export function printPoints(map: MapLayer, base: Pick<Base, "project" | "shapes">): PrintPoint[] {
  const typeName = (id?: string) => map.types?.find((t) => t.id === id)?.name ?? "";
  const out: Omit<PrintPoint, "n">[] = [];

  if (map.kind === "point") {
    for (const i of map.items ?? []) {
      const p = base.project(i.lon, i.lat);
      out.push({ x: p.x, y: p.y, name: i.name, detail: [i.state, i.detail].filter(Boolean).join(" · "), type: i.type, id: i.id });
    }
  } else if (map.kind === "line") {
    for (const l of map.lines ?? []) {
      const p = startOf(l.d);
      if (p) out.push({ ...p, name: l.name, detail: typeName(l.type) });
    }
  } else {
    // States, and region maps shaded by state: one number per state, the type of a region map goes in the key.
    for (const s of base.shapes) {
      const p = centre(s.d);
      if (p) out.push({ ...p, name: s.name, detail: map.kind === "region" ? typeName(map.regions?.[s.id]) : "" });
    }
  }

  return out.map((p, i) => ({ ...p, n: i + 1 }));
}

// The Labelled tab (names and lines on the picture) is for dot maps and the states map on India's outline.
export const labellable = (map: Pick<MapLayer, "kind" | "base" | "items">) => map.base === "india" && (map.kind === "states" || (map.kind === "point" && (map.items?.length ?? 0) > 0));
