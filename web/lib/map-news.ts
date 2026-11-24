import type { MapLayer } from "./maps";

// "In the news": which places of the India maps this week's stories talk about. The places are the names already in
// lib/data/maps (states, cities, rivers, dams, parks, ports and so on), so no geocoding service is involved. A story counts for
// a place when the place's name stands in its title, summary, why-it-matters line or tags as a whole word, spelled with a
// capital (the names are proper nouns, so "son" and "mahi" the common words do not match, "Son" at the start of a line is
// the known miss: see STOP). Pure and server-safe, so Node tests can import it.

export type Story = { id: string; title: string; description?: string; why?: string; tags?: string[]; source: string; time: string };

export type Place = {
  key: string;
  name: string;
  // The titles of the maps the place is on, for the list ("Dams", "Cities").
  on: string[];
  // How it is spelled in a story: the name, the name without "Dam" or "National Park", and older spellings.
  names: string[];
  lon?: number;
  lat?: number;
  // A state is shaded on the outline; a river is drawn as a line.
  shape?: string;
  d?: string;
};

export type Match = { place: Place; count: number; stories: Story[] };

// Maps that stay out: river sources and mouths repeat the river lines, and the monsoon map is by region.
const SKIP = new Set(["rivers", "monsoon"]);

// Older or other spellings that news still uses, by the name as it is in the data.
const ALIASES: Record<string, string[]> = {
  Bengaluru: ["Bangalore"],
  Mumbai: ["Bombay"],
  Kolkata: ["Calcutta"],
  Chennai: ["Madras"],
  Kochi: ["Cochin"],
  Odisha: ["Orissa"],
  Uttarakhand: ["Uttaranchal"],
  Puducherry: ["Pondicherry"],
  Prayagraj: ["Allahabad"],
  Thiruvananthapuram: ["Trivandrum"],
  Ganga: ["Ganges"],
  Kaveri: ["Cauvery"],
  Vadodara: ["Baroda"],
  Visakhapatnam: ["Vizag"],
  "Jammu and Kashmir": ["J&K"],
  "Andaman and Nicobar Islands": ["Andaman and Nicobar", "Andamans"],
};

// Words that are also plain English or other things, so a capital letter is not proof: never matched.
// Park names shortened by "National Park" or "Tiger Reserve" leave a first name or a god (Sanjay, Madhav, Kali) or a common word (Desert).
export const STOP = new Set(["Son", "Ken", "Pass", "Gate", "Bay", "Hills", "Hill", "Island", "Islands", "Lake", "Plateau", "Reserve", "Valley", "Desert", "Sanjay", "Madhav", "Kali", "Valmiki", "Rimo I", "Marine", "Bison"]);

// Rivers that share their name with a person or a god: a story counts only when it says "Krishna river" or "River Krishna".
const NEEDS_RIVER = new Set(["Krishna", "Ravi"]);

const SUFFIX = /\s+(Tiger Reserve|National Park|Wildlife Sanctuary|Sanctuary|Biosphere Reserve|Reserve|Dam|Barrage|Reservoir)$/i;
const MIN_LEN = 4;

// The spellings of one name: with and without a bracketed note and a trailing "Dam" or "National Park".
function spellings(name: string, minLen = MIN_LEN): string[] {
  const bare = name.replace(/\s*\(.*?\)\s*/g, " ").replace(/\s+/g, " ").trim();
  if (NEEDS_RIVER.has(bare)) return [`${bare} River`, `${bare} river`, `River ${bare}`];
  const out = new Set([bare, bare.replace(SUFFIX, "")]);
  for (const a of ALIASES[bare] ?? []) out.add(a);
  return [...out].filter((n) => n.length >= minLen && !STOP.has(n));
}

const norm = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();

// gazetteer: every place of the given India maps (point and line maps, and the states of the outline), one place a name even
// when several maps list it (Mumbai is a city, a capital and a port). A state wins over a city of the same name (Delhi, Goa).
export function gazetteer(layers: MapLayer[], states: { id: string; name: string }[]): Place[] {
  const places = new Map<string, Place>();

  for (const s of states) {
    // A state may have a short name (Goa).
    const names = spellings(s.name, 3);
    if (names.length) places.set(norm(s.name), { key: `state:${s.id}`, name: s.name, on: ["States and union territories"], names, shape: s.id });
  }

  for (const l of layers) {
    if (l.base !== "india" || SKIP.has(l.id) || (l.kind !== "point" && l.kind !== "line")) continue;
    for (const x of [...(l.items ?? []).map((i) => ({ ...i, d: undefined as string | undefined })), ...(l.lines ?? []).map((i) => ({ ...i, lon: undefined, lat: undefined }))]) {
      const names = spellings(x.name);
      if (!names.length) continue;
      const k = norm(x.name.replace(/\s*\(.*?\)\s*/g, " ").replace(SUFFIX, ""));
      const old = places.get(k);
      if (old) {
        if (!old.on.includes(l.title)) old.on.push(l.title);
        if (!old.shape && old.lon === undefined && x.lon !== undefined) Object.assign(old, { lon: x.lon, lat: x.lat });
        if (!old.d && x.d) old.d = x.d;
        continue;
      }
      places.set(k, { key: `${l.id}:${x.id}`, name: x.name.replace(/\s*\(.*?\)\s*/g, " ").trim(), on: [l.title], names, lon: x.lon, lat: x.lat, d: x.d });
    }
  }
  return [...places.values()];
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// Whether a name stands in the text as a whole word (the letters and digits around it are not part of a longer word).
export function mentions(text: string, name: string): boolean {
  return new RegExp(`(?<![\\p{L}\\p{N}])${escape(name)}(?![\\p{L}\\p{N}])`, "u").test(text);
}

// matchStories: for each place the stories that name it, newest first (at most `per` kept, all counted), places with the most
// stories first and then by name. Places no story names are left out.
export function matchStories(places: Place[], stories: Story[], per = 5): Match[] {
  const texts = stories.map((s) => ({ s, t: [s.title, s.description, s.why, ...(s.tags ?? [])].filter(Boolean).join(" \n ") }));
  const out: Match[] = [];
  for (const place of places) {
    const re = new RegExp(`(?<![\\p{L}\\p{N}])(?:${place.names.map(escape).join("|")})(?![\\p{L}\\p{N}])`, "u");
    const hits = texts.filter(({ t }) => re.test(t)).map(({ s }) => s).sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
    if (hits.length) out.push({ place, count: hits.length, stories: hits.slice(0, per) });
  }
  return out.sort((a, b) => b.count - a.count || a.place.name.localeCompare(b.place.name));
}
