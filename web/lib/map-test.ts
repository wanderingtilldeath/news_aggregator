// Test mode of the map pages: pick places from a map, judge where the reader taps, and keep the best scores in their own
// browser (localStorage). Pure, so Node tests can import it.
import type { MapLayer } from "./maps";

export const SCORES_KEY = "pp-maps-scores";
export const TEST_LENGTH = 10;
export const DEFAULT_TOLERANCE_KM = 100;

// One thing to find. A place is found by tapping near it; a state by tapping inside it; the type of a state (its soil or
// climate) by choosing from the legend.
export type Question =
  | { kind: "place"; id: string; ask: string; label: string; hint: string; lon: number; lat: number; note?: string }
  | { kind: "state"; id: string; ask: string; hint: string; stateId: string }
  | { kind: "type"; id: string; ask: string; hint: string; stateId: string; answer: string }
  | { kind: "line"; id: string; ask: string; hint: string; d: string };

// Every question a map can ask. `states` is the outline's list (id and name).
export function questionsFor(map: MapLayer, states: { id: string; name: string }[]): Question[] {
  if (map.kind === "states") return states.map((s) => ({ kind: "state", id: s.id, ask: `Find ${s.name}`, hint: "Tap inside the state or union territory", stateId: s.id }));
  if (map.kind === "region") {
    return states.filter((s) => map.regions?.[s.id]).map((s) => ({ kind: "type", id: s.id, ask: `What is the main ${map.id === "soils" ? "soil" : "climate"} of ${s.name}?`, hint: "", stateId: s.id, answer: map.regions![s.id] }));
  }
  // A map of one kind has only items or only lines; a combined map (lib/map-catalog.ts) may have both.
  const out: Question[] = [];
  if (map.kind === "line" || map.kind === "point") {
    for (const l of map.lines ?? []) out.push({ kind: "line", id: l.id, ask: `Find the ${l.name}`, hint: l.hint ?? (l.type ? `A ${l.type} river` : ""), d: l.d });
    for (const i of map.items ?? []) out.push({ kind: "place", id: i.id, ask: i.ask ?? `Find ${i.name}`, label: i.name, hint: i.state ? `In ${i.state}` : "", lon: i.lon, lat: i.lat, ...(map.category === "history" && i.detail ? { note: i.detail } : {}) });
    return out;
  }
  return [];
}

// n questions chosen at random without repeats (Fisher-Yates on a copy); fewer when the map has fewer. `rand` gives a
// number from 0 up to but not including 1, so a test can pass a fixed one.
export function pickQuestions<T>(all: T[], n: number, rand: () => number): T[] {
  const pool = [...all];
  for (let i = pool.length - 1; i > 0; i--) {
    const j = Math.floor(rand() * (i + 1));
    [pool[i], pool[j]] = [pool[j], pool[i]];
  }
  return pool.slice(0, n);
}

const R = 6371;
const rad = (d: number) => (d * Math.PI) / 180;
// The great-circle distance in whole kilometres.
export function distKm(a: { lon: number; lat: number }, b: { lon: number; lat: number }) {
  const h = Math.sin(rad(b.lat - a.lat) / 2) ** 2 + Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(rad(b.lon - a.lon) / 2) ** 2;
  return Math.round(2 * R * Math.asin(Math.sqrt(h)));
}

// A tap at (lon, lat) for a place question: how far it was, and whether that is near enough.
export function judgePlace(q: Extract<Question, { kind: "place" }>, tap: { lon: number; lat: number }, toleranceKm = DEFAULT_TOLERANCE_KM) {
  const km = distKm(q, tap);
  return { km, correct: km <= toleranceKm };
}

// A tap near a line (a river): the distance in map units (see PROJ in lib/maps.ts: 36 units are one degree of latitude) from the
// point to the nearest part of the path, whose "M x y L x y …" is read here. Taps within LINE_TOLERANCE count.
export const LINE_TOLERANCE = 18;
export function distToPath(d: string, x: number, y: number): number {
  let best = Infinity;
  for (const sub of d.split("M").filter(Boolean)) {
    const pts = sub.split("L").map((p) => p.trim().split(" ").map(Number));
    for (let i = 0; i < pts.length; i++) {
      const [x1, y1] = pts[i];
      const [x2, y2] = pts[i + 1] ?? pts[i];
      const dx = x2 - x1;
      const dy = y2 - y1;
      const len = dx * dx + dy * dy;
      const t = len === 0 ? 0 : Math.max(0, Math.min(1, ((x - x1) * dx + (y - y1) * dy) / len));
      best = Math.min(best, Math.hypot(x - (x1 + t * dx), y - (y1 + t * dy)));
    }
  }

  return best;
}

export type Result = { id: string; ask: string; correct: boolean; km?: number };

export function summarise(results: Result[]) {
  return { score: results.filter((r) => r.correct).length, total: results.length, misses: results.filter((r) => !r.correct) };
}

export type Scores = Record<string, { best: number; total: number; plays: number; at: number }>;

export function parseScores(raw: string): Scores {
  try {
    const v = JSON.parse(raw || "{}");
    if (!v || typeof v !== "object" || Array.isArray(v)) return {};
    return Object.fromEntries(Object.entries(v as Scores).filter(([, s]) => s && Number.isFinite(s.best) && Number.isFinite(s.total) && Number.isFinite(s.plays)));
  } catch {
    return {};
  }
}

// recordScore keeps the best result of a map: more right answers wins, and out of the same number of questions a
// shorter test never replaces a longer one that scored the same share.
export function recordScore(scores: Scores, mapId: string, score: number, total: number, now: number): Scores {
  const old = scores[mapId];
  const better = !old || score / total > old.best / old.total || (score / total === old.best / old.total && total > old.total);
  return { ...scores, [mapId]: { best: better ? score : old.best, total: better ? total : old.total, plays: (old?.plays ?? 0) + 1, at: now } };
}
