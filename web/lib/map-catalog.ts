// The map library's kinds and how a list of maps is narrowed to what a reader asked for. Pure, so Node tests can import it.
import type { MapLayer, MapSummary } from "./maps";

// The kinds of map, in the order the picker shows them (scripts/map-index.mjs keeps the same order).
export const KINDS = [
  { id: "political", label: "India: political", blurb: "States, capitals and union territories" },
  { id: "physical", label: "Physical features", blurb: "Mountains, passes, plateaus and lakes" },
  { id: "rivers", label: "Rivers and dams", blurb: "River systems, sources, mouths and dams" },
  { id: "climate", label: "Climate and soils", blurb: "Soils, climate types and the monsoon" },
  { id: "wildlife", label: "Wildlife and protected areas", blurb: "Parks, reserves, sanctuaries and wetlands" },
  { id: "economy", label: "Economy and infrastructure", blurb: "Ports, airports, power plants and cities" },
  { id: "heritage", label: "Heritage and culture", blurb: "World Heritage Sites and cultural places" },
  { id: "history", label: "History", blurb: "Battles, Indus Valley sites, early kingdoms and the freedom movement" },
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

// ---- combined maps: two to four maps of one outline drawn together (rivers with dams, ports with airports) ----

// The pairs worth offering as links; a reader can also pick their own on /maps/combine.
export const COMBOS = [
  { ids: ["river-systems", "dams"], label: "Rivers and dams", blurb: "Which river each big dam sits on" },
  { ids: ["ranges", "peaks", "passes"], label: "Ranges, peaks and passes", blurb: "The mountain belt on one map" },
  { ids: ["national-parks", "tiger", "biosphere"], label: "Protected areas", blurb: "National parks, tiger reserves and biosphere reserves" },
  { ids: ["ports", "airports"], label: "Ports and airports", blurb: "Where goods and people enter" },
  { ids: ["power", "dams"], label: "Power and dams", blurb: "Power plants beside the dams" },
  { ids: ["world-seas", "world-straits"], label: "Seas and straits", blurb: "The world's waters together" },
] as const;

export const MAX_COMBINED = 4;
// One colour a layer, told apart by more than hue alone (the dots of one layer share the colour).
const PALETTE = ["#1d5fb8", "#c4511e", "#2f7d5b", "#8a4fbf"];

// A map can be combined when it is dots or lines; states and regions are shaded by state and do not mix.
export const combinable = (m: Pick<MapLayer, "kind">) => m.kind === "point" || m.kind === "line";

// combine draws the given maps as one: each layer's places keep their names and get the layer's own colour (the legend names
// the layer), so Study and Test treat the result like any other map. It answers an error in words for a choice that cannot be
// made (fewer than two, too many, or two outlines).
export function combine(layers: MapLayer[]): { map: MapLayer; error?: undefined } | { map?: undefined; error: string } {
  if (layers.length < 2) return { error: "Choose at least two maps to combine." };
  if (layers.length > MAX_COMBINED) return { error: `Choose at most ${MAX_COMBINED} maps to combine.` };
  if (layers.some((l) => !combinable(l))) return { error: "Maps shaded by state (soils, climate, states) cannot be combined." };
  if (new Set(layers.map((l) => l.base)).size > 1) return { error: "A map of India and a map of the world cannot share one outline." };

  const items = layers.flatMap((l) => (l.items ?? []).map((i) => ({ ...i, id: `${l.id}:${i.id}`, type: l.id })));
  const lines = layers.flatMap((l) => (l.lines ?? []).map((x) => ({ ...x, id: `${l.id}:${x.id}`, type: l.id, hint: l.title })));
  const tolerances = layers.filter((l) => l.kind === "point").map((l) => l.tolerance_km ?? 100);

  return {
    map: {
      id: `combine:${layers.map((l) => l.id).sort().join("+")}`,
      title: layers.map((l) => l.title).join(" + "),
      category: layers[0].category,
      blurb: `${layers.map((l) => l.title).join(", ")}, drawn together on one outline.`,
      kind: items.length ? "point" : "line",
      base: layers[0].base,
      source: [...new Set(layers.map((l) => l.source))].join("; "),
      licence: [...new Set(layers.map((l) => l.licence))].join("; "),
      asOf: layers.map((l) => l.asOf).sort().at(-1) ?? "",
      approx: layers.some((l) => l.approx) || undefined,
      color: PALETTE[0],
      tolerance_km: tolerances.length ? Math.max(...tolerances) : undefined,
      types: layers.map((l, n) => ({ id: l.id, name: l.title, color: PALETTE[n], also: l.blurb })),
      items: items.length ? items : undefined,
      lines: lines.length ? lines : undefined,
    },
  };
}
