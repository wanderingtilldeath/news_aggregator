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

// The ready-made layers (lib/data/map-layers.json).
export type LayerItem = { id: string; name: string; lon: number; lat: number; state: string; detail?: string };
export type LayerType = { id: string; name: string; color: string; also: string };
export type MapLayer = {
  id: string;
  title: string;
  group: string;
  kind: "point" | "region";
  source: string;
  asOf: string;
  note?: string;
  approx?: boolean;
  color?: string; // point layers: the dot colour
  items?: LayerItem[]; // point layers
  types?: LayerType[]; // region layers: the legend
  regions?: Record<string, string>; // region layers: state id -> type id
};
