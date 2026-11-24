// The map practice page's own markers: places a student marks on the India map, each with a note, kept in their own
// browser (localStorage). Nothing is sent anywhere. This file has no server code, so client components and Node tests can
// import it.

export type Marker = { id: string; lon: number; lat: number; label: string; note: string };

export const MAPS_KEY = "pp-maps";
export const MAX_MARKERS = 200;
export const MAX_LABEL = 60;
export const MAX_NOTE = 500;

// The projection of lib/data/india.json (scripts/india-data.mjs has the same numbers): equirectangular, with the east-west
// scale shrunk by k so shapes look right at India's latitudes. x grows east, y grows south.
export const PROJ = { lon0: 68, lat1: 37.2, k: 0.92, s: 36 };
export const BOUNDS = { minLon: 68, maxLon: 97.5, minLat: 6.5, maxLat: 37.2 };

export const project = (lon: number, lat: number) => ({ x: (lon - PROJ.lon0) * PROJ.k * PROJ.s, y: (PROJ.lat1 - lat) * PROJ.s });
export const unproject = (x: number, y: number) => ({ lon: x / (PROJ.k * PROJ.s) + PROJ.lon0, lat: PROJ.lat1 - y / PROJ.s });

// The projection of lib/data/world.json (scripts/world-data.mjs has the same numbers): equirectangular, 3 units a degree,
// from 85 N down to 60 S (Antarctica is left off). x grows east, y grows south.
export const WORLD = { s: 3, top: 85, bottom: -60 };
export const projectWorld = (lon: number, lat: number) => ({ x: (lon + 180) * WORLD.s, y: (WORLD.top - lat) * WORLD.s });
export const unprojectWorld = (x: number, y: number) => ({ lon: x / WORLD.s - 180, lat: WORLD.top - y / WORLD.s });
export const inWorld = (lon: number, lat: number) => lon >= -180 && lon <= 180 && lat >= WORLD.bottom && lat <= WORLD.top;

export const inIndia = (lon: number, lat: number) => lon >= BOUNDS.minLon && lon <= BOUNDS.maxLon && lat >= BOUNDS.minLat && lat <= BOUNDS.maxLat;

const clip = (s: unknown, max: number) => (typeof s === "string" ? s.trim().slice(0, max) : "");

// parseMarkers reads the stored text. Anything unreadable, or outside the map, is dropped rather than shown.
export function parseMarkers(raw: string): Marker[] {
  try {
    const list = JSON.parse(raw || "[]");
    if (!Array.isArray(list)) return [];
    return list
      .filter((m) => m && typeof m.id === "string" && Number.isFinite(m.lon) && Number.isFinite(m.lat) && inIndia(m.lon, m.lat))
      .map((m) => ({ id: m.id, lon: m.lon, lat: m.lat, label: clip(m.label, MAX_LABEL), note: clip(m.note, MAX_NOTE) }))
      .slice(0, MAX_MARKERS);
  } catch {
    return [];
  }
}

// addMarker puts the marker first. A marker whose id is already there (a ready-made place added twice), or one past the
// cap, leaves the list as it was.
export function addMarker(list: Marker[], m: Marker): Marker[] {
  if (list.length >= MAX_MARKERS || list.some((x) => x.id === m.id) || !inIndia(m.lon, m.lat)) return list;
  return [{ ...m, label: clip(m.label, MAX_LABEL), note: clip(m.note, MAX_NOTE) }, ...list];
}

export function updateMarker(list: Marker[], id: string, patch: Partial<Pick<Marker, "label" | "note">>): Marker[] {
  return list.map((m) => (m.id === id ? { ...m, label: patch.label === undefined ? m.label : clip(patch.label, MAX_LABEL), note: patch.note === undefined ? m.note : clip(patch.note, MAX_NOTE) } : m));
}

export const removeMarker = (list: Marker[], id: string): Marker[] => list.filter((m) => m.id !== id);

// The ready-made maps (lib/data/maps/<id>.json, listed by lib/data/maps/index.json).
// ask, when present, is the question Test mode puts (the name would give the answer away: "Find the capital of France").
// era, on a history map, is the period the place belongs to; the viewer then offers the eras as chips (erasOf, forEra below).
export type LayerItem = { id: string; name: string; lon: number; lat: number; state: string; detail?: string; type?: string; ask?: string; era?: string };
export type LayerType = { id: string; name: string; color: string; also: string };
export type MapLine = { id: string; name: string; d: string; type?: string; hint?: string };
// How much of the real list a map holds: "complete" is every place of an official list (of = its official count, as of the date),
// "selection" is the best-known places of a longer or open-ended list (of = how many there are in all; about is set when that is only roughly known).
export type Coverage = { type: "complete" | "selection"; of: number; asOf: string; about?: boolean };

// The line shown under a map and on its card.
export const coverageLine = (c: Coverage, shown: number) => (c.type === "complete" ? `All ${c.of}, as of ${c.asOf}` : `${shown} of ${c.about ? "about " : ""}${c.of}`);

export type MapLayer = {
  id: string;
  title: string;
  // The kind of map the picker groups it under (KINDS in lib/map-catalog.ts) and one line on what it teaches.
  category: string;
  blurb: string;
  // How it is drawn: dots (point), states shaded by type (region), river lines (line), or just the states (states).
  kind: "point" | "region" | "line" | "states";
  // Which outline it sits on; a world map uses the world outline.
  base: "india" | "world";
  source: string;
  licence: string;
  asOf: string;
  note?: string;
  approx?: boolean;
  color?: string; // point and line maps: the colour
  tolerance_km?: number; // Test mode: how near a tap must be to count (100 when absent)
  items?: LayerItem[]; // point maps
  types?: LayerType[]; // region maps: the legend (and line maps: the kinds of river)
  regions?: Record<string, string>; // region maps: state id -> type id
  lines?: MapLine[]; // line maps
  coverage?: Coverage;
};

// What the library page needs of a map, without its items (lib/data/maps/index.json).
export type MapSummary = Pick<MapLayer, "id" | "title" | "category" | "blurb" | "kind" | "base" | "source" | "licence" | "asOf" | "coverage"> & {
  approx?: boolean;
  color?: string;
  count: number;
  dots?: [number, number][];
  shade?: Record<string, string>;
  paths?: string[];
};

// The eras a map is told in, in the order they first appear (none when it has no eras or only one).
export function erasOf(map: Pick<MapLayer, "items">): string[] {
  const eras = [...new Set((map.items ?? []).flatMap((i) => (i.era ? [i.era] : [])))];
  return eras.length > 1 ? eras : [];
}

// The map with only the places of one era ("" keeps all).
export function forEra<T extends Pick<MapLayer, "items">>(map: T, era: string): T {
  return era && map.items ? { ...map, items: map.items.filter((i) => i.era === era) } : map;
}
