// The map library's kinds and how a list of maps is narrowed to what a reader asked for. Pure, so Node tests can import it.
import type { MapSummary } from "./maps";

// The kinds of map, in the order the picker shows them (scripts/map-index.mjs keeps the same order).
export const KINDS = [
  { id: "political", label: "India: political", blurb: "States, capitals and union territories" },
  { id: "physical", label: "Physical features", blurb: "Mountains, passes, plateaus and lakes" },
  { id: "rivers", label: "Rivers and dams", blurb: "River systems, sources, mouths and dams" },
  { id: "climate", label: "Climate and soils", blurb: "Soils, climate types and the monsoon" },
  { id: "wildlife", label: "Wildlife and protected areas", blurb: "Parks, reserves, sanctuaries and wetlands" },
  { id: "economy", label: "Economy and infrastructure", blurb: "Ports, airports, power plants and cities" },
  { id: "heritage", label: "Heritage and culture", blurb: "World Heritage Sites and cultural places" },
  { id: "world-seas", label: "World: straits and seas", blurb: "Straits, seas, gulfs and channels" },
  { id: "world-capitals", label: "World: capitals", blurb: "Countries and their capitals" },
  { id: "world-land", label: "World: mountains and deserts", blurb: "Ranges, peaks and deserts of the world" },
] as const;

export type KindId = (typeof KINDS)[number]["id"];

export const kindLabel = (id: string) => KINDS.find((k) => k.id === id)?.label ?? id;

// How many maps each kind holds (kinds with none are left out).
export function countByKind(maps: MapSummary[]): { id: KindId; label: string; blurb: string; count: number }[] {
  return KINDS.map((k) => ({ ...k, count: maps.filter((m) => m.category === k.id).length })).filter((k) => k.count > 0);
}

// The maps of one kind ("" for all) whose title, blurb or kind label has every word of the query, in the order given.
export function filterMaps(maps: MapSummary[], kind: string, query: string): MapSummary[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return maps.filter((m) => {
    if (kind && m.category !== kind) return false;
    const hay = `${m.title} ${m.blurb} ${kindLabel(m.category)}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}
