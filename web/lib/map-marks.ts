import { inIndia, inWorld, parseMarkers } from "./maps.ts";

// What a reader keeps on the maps, in their own browser (and in their account when logged in): the places they star to revise.
// Each star belongs to one map, so one on the tiger map does not clutter the dams map. Pure, so Node tests can import it.

export type Mark = {
  id: string;
  // The map it was made on.
  map: string;
  base: "india" | "world";
  lon: number;
  lat: number;
  name: string;
  // The place's id on that map (a place or a river of it).
  item: string;
  from: "star" | "test";
  at: number;
};

export const MARKS_KEY = "pp-maps-marks";
export const MAX_MARKS = 300;
export const MAX_NAME = 60;

const clip = (s: unknown, max: number) => (typeof s === "string" ? s.trim().slice(0, max) : "");
const inside = (base: "india" | "world", lon: number, lat: number) => (base === "world" ? inWorld(lon, lat) : inIndia(lon, lat));

const clean = (m: unknown): Mark | null => {
  const x = m as Partial<Mark> | null;
  // Free notes made by an earlier version have no place of a map: they are dropped.
  if (!x || typeof x.id !== "string" || !x.id || x.id.length > 200 || typeof x.lon !== "number" || typeof x.lat !== "number" || typeof x.item !== "string" || !x.item) return null;
  const base = x.base === "world" ? "world" : "india";
  if (!inside(base, x.lon, x.lat)) return null;
  const from = x.from === "test" ? "test" : "star";
  return { id: x.id, map: clip(x.map, 100), base, lon: x.lon, lat: x.lat, name: clip(x.name, MAX_NAME) || "My place", item: x.item.slice(0, 120), from, at: Number.isFinite(x.at) ? Number(x.at) : 0 };
};

// parseMarks reads the stored text (newest first). Unreadable or out-of-map entries are dropped. With nothing stored yet, the
// markers of the earlier single-list version (`legacy`, key pp-maps) are carried over when they were made from a map's list
// (they keep their map); its free notes are not.
export function parseMarks(raw: string, legacy = ""): Mark[] {
  let list: unknown[] = [];
  if (raw) {
    try {
      const v = JSON.parse(raw);
      list = Array.isArray(v) ? v : [];
    } catch {
      list = [];
    }
  } else {
    list = parseMarkers(legacy).flatMap((m) => {
      const i = m.id.indexOf(":");
      return i > 0 ? [{ id: m.id, map: m.id.slice(0, i), base: "india", lon: m.lon, lat: m.lat, name: m.label, item: m.id.slice(i + 1), from: "star", at: 0 }] : [];
    });
  }
  return list.map(clean).filter((m): m is Mark => m !== null).sort((a, b) => b.at - a.at).slice(0, MAX_MARKS);
}

export const starId = (map: string, item: string) => `${map}:${item}`;
export const isStarred = (marks: Mark[], map: string, item: string) => marks.some((m) => m.id === starId(map, item));

type Spot = { id: string; name: string; lon: number; lat: number };

// Star a place of a map, or take the star off. A full list changes nothing.
export function toggleStar(marks: Mark[], map: string, base: "india" | "world", spot: Spot, now: number): Mark[] {
  const id = starId(map, spot.id);
  if (marks.some((m) => m.id === id)) return marks.filter((m) => m.id !== id);
  if (marks.length >= MAX_MARKS) return marks;
  return [{ id, map, base, lon: spot.lon, lat: spot.lat, name: clip(spot.name, MAX_NAME), item: spot.id, from: "star", at: now }, ...marks];
}

// Star the places a test was missed on (those not already starred), marked as coming from a test.
export function starMissed(marks: Mark[], map: string, base: "india" | "world", spots: Spot[], now: number): Mark[] {
  let out = marks;
  for (const s of spots) {
    if (out.length >= MAX_MARKS) break;
    if (!out.some((m) => m.id === starId(map, s.id))) out = [{ id: starId(map, s.id), map, base, lon: s.lon, lat: s.lat, name: clip(s.name, MAX_NAME), item: s.id, from: "test", at: now }, ...out];
  }
  return out;
}

export const removeMark = (marks: Mark[], id: string): Mark[] => marks.filter((m) => m.id !== id);

// Put a removed mark back (the Undo), in its place by time.
export function restoreMark(marks: Mark[], m: Mark): Mark[] {
  if (marks.some((x) => x.id === m.id) || marks.length >= MAX_MARKS) return marks;
  return [...marks, m].sort((a, b) => b.at - a.at);
}

// The marks to show on a map: its own, or with `all` every mark on the same outline.
export function marksFor(marks: Mark[], map: string, base: "india" | "world", all = false): Mark[] {
  return marks.filter((m) => m.base === base && (all || m.map === map));
}

export const clearMap = (marks: Mark[], map: string): Mark[] => marks.filter((m) => m.map !== map);

// The ids of the places starred on a map.
export function starredItems(marks: Mark[], map: string): Set<string> {
  return new Set(marks.filter((m) => m.map === map).map((m) => m.item));
}

// The map with only its starred places (for "test my starred places").
export function onlyStarred<T extends { items?: { id: string }[]; lines?: { id: string }[] }>(map: T, stars: Set<string>): T {
  return { ...map, ...(map.items ? { items: map.items.filter((i) => stars.has(i.id)) } : {}), ...(map.lines ? { lines: map.lines.filter((l) => stars.has(l.id)) } : {}) };
}
