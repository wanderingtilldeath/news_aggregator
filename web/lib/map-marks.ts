import { inIndia, inWorld, parseMarkers } from "./maps.ts";

// What a reader keeps on the maps, in their own browser (and in their account when logged in): places they star to revise
// (with a memory hook of their own) and free notes dropped at a spot of a map. Each belongs to one map, so a note on the tiger
// map does not clutter the dams map. Pure, so Node tests can import it.

export type Mark = {
  id: string;
  // The map it was made on ("" for a note made before notes belonged to a map: it shows on every India map).
  map: string;
  base: "india" | "world";
  lon: number;
  lat: number;
  name: string;
  note: string;
  // The place's id on that map when it was starred from the list or by a missed test question; absent for a free note.
  item?: string;
  from: "star" | "test" | "note";
  at: number;
};

export const MARKS_KEY = "pp-maps-marks";
export const MAX_MARKS = 300;
export const MAX_NAME = 60;
export const MAX_NOTE = 500;

const clip = (s: unknown, max: number) => (typeof s === "string" ? s.trim().slice(0, max) : "");
const inside = (base: "india" | "world", lon: number, lat: number) => (base === "world" ? inWorld(lon, lat) : inIndia(lon, lat));

const clean = (m: unknown): Mark | null => {
  const x = m as Partial<Mark> | null;
  if (!x || typeof x.id !== "string" || !x.id || x.id.length > 200 || typeof x.lon !== "number" || typeof x.lat !== "number") return null;
  const base = x.base === "world" ? "world" : "india";
  if (!inside(base, x.lon, x.lat)) return null;
  const from = x.from === "test" || x.from === "star" ? x.from : "note";
  return { id: x.id, map: clip(x.map, 100), base, lon: x.lon, lat: x.lat, name: clip(x.name, MAX_NAME) || "My place", note: clip(x.note, MAX_NOTE), ...(typeof x.item === "string" && x.item ? { item: x.item.slice(0, 120) } : {}), from, at: Number.isFinite(x.at) ? Number(x.at) : 0 };
};

// parseMarks reads the stored text (newest first). Unreadable or out-of-map entries are dropped. With nothing stored yet, the
// notes of the earlier single-list version (`legacy`, key pp-maps) are carried over: one made from a map's list keeps its map,
// the rest become notes that show on every India map.
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
    list = parseMarkers(legacy).map((m) => {
      const i = m.id.indexOf(":");
      return { id: m.id, map: i > 0 ? m.id.slice(0, i) : "", base: "india", lon: m.lon, lat: m.lat, name: m.label, note: m.note, ...(i > 0 ? { item: m.id.slice(i + 1) } : {}), from: i > 0 ? "star" : "note", at: 0 };
    });
  }
  return list.map(clean).filter((m): m is Mark => m !== null).sort((a, b) => b.at - a.at).slice(0, MAX_MARKS);
}

export const starId = (map: string, item: string) => `${map}:${item}`;
export const isStarred = (marks: Mark[], map: string, item: string) => marks.some((m) => m.id === starId(map, item));

type Spot = { id: string; name: string; lon: number; lat: number };

// Star a place of a map, or take the star off (its note goes with it). A repeat or a full list changes nothing.
export function toggleStar(marks: Mark[], map: string, base: "india" | "world", spot: Spot, now: number): Mark[] {
  const id = starId(map, spot.id);
  if (marks.some((m) => m.id === id)) return marks.filter((m) => m.id !== id);
  if (marks.length >= MAX_MARKS) return marks;
  return [{ id, map, base, lon: spot.lon, lat: spot.lat, name: clip(spot.name, MAX_NAME), note: "", item: spot.id, from: "star", at: now }, ...marks];
}

// Star the places a test was missed on (those not already starred), marked as coming from a test.
export function starMissed(marks: Mark[], map: string, base: "india" | "world", spots: Spot[], now: number): Mark[] {
  let out = marks;
  for (const s of spots) {
    if (out.length >= MAX_MARKS) break;
    if (!out.some((m) => m.id === starId(map, s.id))) out = [{ id: starId(map, s.id), map, base, lon: s.lon, lat: s.lat, name: clip(s.name, MAX_NAME), note: "", item: s.id, from: "test", at: now }, ...out];
  }
  return out;
}

// A free note at a spot of the map.
export function addNote(marks: Mark[], m: { id: string; map: string; base: "india" | "world"; lon: number; lat: number; name: string; note: string }, now: number): Mark[] {
  if (marks.length >= MAX_MARKS || marks.some((x) => x.id === m.id) || !inside(m.base, m.lon, m.lat)) return marks;
  return [{ ...m, name: clip(m.name, MAX_NAME) || "My place", note: clip(m.note, MAX_NOTE), from: "note", at: now }, ...marks];
}

export function updateMark(marks: Mark[], id: string, patch: { name?: string; note?: string }): Mark[] {
  return marks.map((m) => (m.id === id ? { ...m, name: patch.name === undefined ? m.name : clip(patch.name, MAX_NAME), note: patch.note === undefined ? m.note : clip(patch.note, MAX_NOTE) } : m));
}

export const removeMark = (marks: Mark[], id: string): Mark[] => marks.filter((m) => m.id !== id);

// Put a removed mark back (the Undo), in its place by time.
export function restoreMark(marks: Mark[], m: Mark): Mark[] {
  if (marks.some((x) => x.id === m.id) || marks.length >= MAX_MARKS) return marks;
  return [...marks, m].sort((a, b) => b.at - a.at);
}

// The marks to show on a map: its own (and the old unscoped notes on an India map), or with `all` every mark on the same outline.
export function marksFor(marks: Mark[], map: string, base: "india" | "world", all = false): Mark[] {
  return marks.filter((m) => m.base === base && (all || m.map === map || (m.map === "" && base === "india")));
}

export const clearMap = (marks: Mark[], map: string): Mark[] => marks.filter((m) => m.map !== map);

// The ids of the places starred on a map.
export function starredItems(marks: Mark[], map: string): Set<string> {
  return new Set(marks.filter((m) => m.map === map && m.item).map((m) => m.item!));
}

// The map with only its starred places (for "test my starred places").
export function onlyStarred<T extends { items?: { id: string }[]; lines?: { id: string }[] }>(map: T, stars: Set<string>): T {
  return { ...map, ...(map.items ? { items: map.items.filter((i) => stars.has(i.id)) } : {}), ...(map.lines ? { lines: map.lines.filter((l) => stars.has(l.id)) } : {}) };
}

// The notes as plain text to save: one block a mark, with the map's title.
export function exportText(marks: Mark[], titleOf: (map: string) => string): string {
  if (!marks.length) return "No notes yet.\n";
  return marks.map((m) => `${m.name} (${titleOf(m.map) || "India map"})${m.from === "test" ? " [missed in a test]" : ""}\n${m.note || "(no note)"}\n`).join("\n");
}
