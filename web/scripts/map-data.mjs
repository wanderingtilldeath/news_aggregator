// Builds the ready-made India maps of lib/data/maps from open data (run by hand, commit the output, then run
// scripts/map-index.mjs). Needs the network. One map or several can be built by id:
//   node scripts/map-data.mjs                 every map below
//   node scripts/map-data.mjs peaks lakes     only these
// Sources, and their licences (shown on each map's page):
//   Wikidata          CC0         query.wikidata.org: peaks, ranges, plateaus, lakes, waterfalls, glaciers, dams, airports,
//                                 cities, state capitals, World Heritage Sites
//   Natural Earth     public domain  rivers (ne_10m_rivers_lake_centerlines, from the natural-earth-vector repository)
//   WRI Global Power Plant Database  CC BY 4.0  power plants of 1,000 MW and more
// No coaching site, textbook or other copyrighted map is used. Each place is put in the state that contains it (or, for a
// coastal or island place, the nearest within about 40 km) using the outline in lib/data/india.json, and dropped when it
// is in none. The projection constants must match PROJ in lib/maps.ts (lib/maps.test.ts checks it).
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const PROJ = { lon0: 68, lat1: 37.2, k: 0.92, s: 36 }; // x = (lon - lon0) * k * s, y = (lat1 - lat) * s
const proj = (lon, lat) => [(lon - PROJ.lon0) * PROJ.k * PROJ.s, (PROJ.lat1 - lat) * PROJ.s];
const OUT = new URL("../lib/data/maps/", import.meta.url);
const CACHE = join(tmpdir(), "pp-map-data");
mkdirSync(CACHE, { recursive: true });
const UA = "PostpaperMapData/1.0 (https://www.postpaper.in; one-off build of open-data revision maps)";
const MIN_ITEMS = 8;
const today = new Date().toISOString().slice(0, 7);

// ---- the outline, for "which state is this place in" -------------------------------------------------------------------
const india = JSON.parse(readFileSync(new URL("../lib/data/india.json", import.meta.url), "utf8"));
const polys = india.states.map((s) => ({
  name: s.name,
  rings: s.d.split("M").filter(Boolean).map((r) => r.replace("Z", "").split("L").map((p) => p.split(" ").map(Number))),
}));
const inRing = (ring, x, y) => {
  let inside = false;
  for (let i = 0, j = ring.length - 1; i < ring.length; j = i++) {
    const [xi, yi] = ring[i];
    const [xj, yj] = ring[j];
    if (yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi) inside = !inside;
  }
  return inside;
};
const NEAR = 0.4 * PROJ.s; // map units: about 0.4 degrees, 40 km
function stateOf(lon, lat) {
  const [x, y] = proj(lon, lat);
  for (const p of polys) if (p.rings.some((r) => inRing(r, x, y))) return p.name;
  let best = null;
  for (const p of polys) for (const r of p.rings) for (const [px, py] of r) {
    const d = Math.hypot(px - x, py - y);
    if (d < NEAR && (!best || d < best.d)) best = { d, name: p.name };
  }
  return best?.name ?? null;
}

// ---- helpers ----------------------------------------------------------------------------------------------------------------
const slug = (s) => s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
const round = (n, d = 3) => Math.round(n * 10 ** d) / 10 ** d;

async function get(url, file, init = {}) {
  const path = join(CACHE, file);
  if (existsSync(path)) return readFileSync(path, "utf8");
  let res;
  // Wikidata answers 429 when asked too fast: wait and ask again, three times at most.
  for (let attempt = 0; attempt < 4; attempt++) {
    res = await fetch(url, { ...init, headers: { "User-Agent": UA, ...(init.headers ?? {}) }, signal: AbortSignal.timeout(180_000) });
    if (res.status !== 429) break;
    await new Promise((r) => setTimeout(r, 30_000 * (attempt + 1)));
  }
  if (!res.ok) throw new Error(`${url.slice(0, 80)}…: ${res.status}`);
  const text = await res.text();
  writeFileSync(path, text);
  return text;
}

async function sparql(query, name) {
  const url = "https://query.wikidata.org/sparql?format=json&query=" + encodeURIComponent(query);
  const text = await get(url, `wd-${name}.json`, { headers: { Accept: "application/sparql-results+json" } });
  return JSON.parse(text).results.bindings;
}

const wkt = (s) => {
  const m = /Point\(([-0-9.]+) ([-0-9.]+)\)/.exec(s);
  return m ? { lon: Number(m[1]), lat: Number(m[2]) } : null;
};

// Rows of {id, name, lon, lat, sl, extra} from a Wikidata query: the label must be English text (not a bare Q-number), one
// row per item (the first), and the place must sit in a state.
function places(rows, { detail } = {}) {
  const seen = new Map();
  for (const r of rows) {
    const id = r.x.value.split("/").pop();
    // "Khandadhar Falls, Sundagarh" is "Khandadhar Falls"; a name that is a protected area or a forest is not what is meant.
    const name = r.xLabel?.value?.split(",")[0].trim();
    const at = r.c && wkt(r.c.value);
    if (name && /\b(reserve|reserved|sanctuary|forest|plains)\b|\(/i.test(name)) continue;
    if (!name || /^Q\d+$/.test(name) || !at) continue;
    const row = seen.get(id) ?? { id, name, ...at, sl: Number(r.sl?.value ?? 0), vals: {} };
    for (const k of ["elev", "year", "pop", "cap"]) if (r[k]) row.vals[k] = Math.max(row.vals[k] ?? 0, Number(r[k].value) || 0);
    if (r.capLabel) row.vals.capLabel = r.capLabel.value;
    seen.set(id, row);
  }
  const out = [];
  for (const row of seen.values()) {
    const state = stateOf(row.lon, row.lat);
    if (!state) continue;
    out.push({ id: slug(row.name), name: row.name, lon: round(row.lon), lat: round(row.lat), state, ...(detail ? { detail: detail(row.vals) } : {}), _sl: row.sl, _v: row.vals });
  }
  // The same name twice (two places called "Kali"): keep the better known.
  const byId = new Map();
  for (const p of out.sort((a, b) => b._sl - a._sl)) if (!byId.has(p.id)) byId.set(p.id, p);
  return [...byId.values()];
}
// Drop the helper fields (sitelink count and raw values, which start with an underscore) before writing.
const strip = (list) => list.map((p) => Object.fromEntries(Object.entries(p).filter(([k]) => !k.startsWith("_"))));

const IND = "wd:Q668";
const q = (body, vars = "", order = "?sl", limit = 400) => `SELECT ?x ?xLabel ?c ?sl ${vars} WHERE { ${body} ?x wdt:P625 ?c . ?x wikibase:sitelinks ?sl . SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . } } ORDER BY DESC(${order}) LIMIT ${limit}`;

// ---- the maps -------------------------------------------------------------------------------------------------------------------
const WD = { source: "Wikidata, queried for items in India with a position and an English label", licence: "CC0 (Wikidata)", asOf: today };
const defs = {
  peaks: async () => {
    // psn: is the height normalised to metres (a plain value can be in feet).
    const rows = await sparql(q(`?x wdt:P31 wd:Q8502 ; wdt:P17 ${IND} ; p:P2044/psn:P2044/wikibase:quantityAmount ?elev .`, "?elev", "?sl", 300), "peaks2");
    const list = places(rows, { detail: (v) => (v.elev ? `${Math.round(v.elev).toLocaleString("en")} m` : "") }).filter((p) => p._v.elev >= 2000 && !/hills?$/i.test(p.name));
    return { ...WD, id: "peaks", title: "Mountain peaks", category: "physical", blurb: "The best-known peaks of the Himalaya and the peninsula, with their height.", color: "#7a4a9a", tolerance_km: 150, items: strip(list.sort((a, b) => b._sl - a._sl).slice(0, 40).sort((a, b) => b._v.elev - a._v.elev)) };
  },
  ranges: async () => {
    const rows = await sparql(q(`?x wdt:P31 wd:Q46831 ; wdt:P17 ${IND} .`, "", "?sl", 120), "ranges");
    return { ...WD, id: "ranges", title: "Mountain ranges and hills", category: "physical", blurb: "The ranges of the Himalaya, the Western and Eastern Ghats and the peninsula. Dots are approximate centres.", approx: true, color: "#8a5a2a", tolerance_km: 250, items: strip(places(rows).slice(0, 35)) };
  },
  plateaus: async () => {
    const rows = await sparql(q(`?x wdt:P31/wdt:P279* wd:Q75520 ; wdt:P17 ${IND} .`, "", "?sl", 200), "plateaus2");
    // Only items that are named plateaus (Wikidata also has hamlets and meadows under this class), and the well-known ones it lacks,
    // added by hand from public geography facts at approximate centres.
    const list = places(rows).filter((p) => /plateau/i.test(p.name) || p.id === "wynaad");
    const extra = [["Mysore Plateau", 12.9, 76.6, "Karnataka"], ["Telangana Plateau", 17.9, 79.0, "Telangana"], ["Bundelkhand Upland", 25.4, 79.0, "Uttar Pradesh"], ["Baghelkhand Plateau", 24.0, 81.5, "Madhya Pradesh"], ["Marwar Plateau", 25.5, 73.5, "Rajasthan"], ["Mewar Plateau", 24.6, 73.7, "Rajasthan"], ["Hazaribagh Plateau", 24.0, 85.4, "Jharkhand"], ["Changthang Plateau", 33.5, 79.0, "Ladakh"]];
    for (const [name, lat, lon, state] of extra) if (!list.some((l) => l.id === slug(name))) list.push({ id: slug(name), name, lon, lat, state, _sl: 1 });
    return { ...WD, id: "plateaus", title: "Plateaus", category: "physical", blurb: "The Deccan, Malwa, Chota Nagpur and other plateaus. Dots are approximate centres.", note: "Several centres are placed by hand from public geography facts and are rough.", approx: true, color: "#b07a2a", tolerance_km: 250, items: strip(list.sort((a, b) => b._sl - a._sl).slice(0, 20)) };
  },
  lakes: async () => {
    const rows = await sparql(q(`?x wdt:P31 wd:Q23397 ; wdt:P17 ${IND} .`, "", "?sl", 200), "lakes");
    // Two famous ones are not "lakes" on Wikidata (Chilika is a lagoon, Sambhar a salt lake); their places are public facts.
    const extra = [{ id: "chilika-lake", name: "Chilika Lake", lon: 85.32, lat: 19.72, state: "Odisha", _sl: 99 }, { id: "sambhar-lake", name: "Sambhar Lake", lon: 75.0, lat: 26.97, state: "Rajasthan", _sl: 98 }];
    const list = places(rows);
    for (const e of extra) if (!list.some((l) => l.id === e.id)) list.push(e);
    // Reservoirs and city tanks are not what a lakes map is for.
    for (const id of ["powai-lake", "bellandur-lake", "tulsi-lake", "ana-sagar-lake", "sasthamcotta-lake", "mansar-lake", "nako-lake"]) list.splice(0, list.length, ...list.filter((l) => l.id !== id));
    return { ...WD, id: "lakes", title: "Lakes", category: "physical", blurb: "Natural lakes and lagoons of India, from Wular and Dal in the north to Chilika and Pulicat on the coast.", color: "#1d8a9c", tolerance_km: 120, items: strip(list.sort((a, b) => b._sl - a._sl).slice(0, 35)) };
  },
  waterfalls: async () => {
    const rows = await sparql(q(`?x wdt:P31 wd:Q34038 ; wdt:P17 ${IND} .`, "", "?sl", 120), "waterfalls");
    return { ...WD, id: "waterfalls", title: "Waterfalls", category: "physical", blurb: "Jog, Dudhsagar, Athirappilly and other well-known falls.", color: "#2f6fbf", tolerance_km: 100, items: strip(places(rows).slice(0, 20)) };
  },
  glaciers: async () => {
    const rows = await sparql(q(`?x wdt:P31 wd:Q35666 ; wdt:P17 ${IND} .`, "", "?sl", 80), "glaciers");
    return { ...WD, id: "glaciers", title: "Glaciers", category: "physical", blurb: "Siachen, Gangotri and the other glaciers of the Himalaya and Karakoram.", color: "#4a90b8", tolerance_km: 150, items: strip(places(rows).slice(0, 15)) };
  },
  dams: async () => {
    const rows = await sparql(q(`?x wdt:P31/wdt:P279* wd:Q12323 ; wdt:P17 ${IND} .`, "", "?sl", 300), "dams2");
    return { ...WD, id: "dams", title: "Major dams", category: "rivers", blurb: "Bhakra, Hirakud, Tehri, Sardar Sarovar and the other great dams, by the river they hold.", color: "#1d5fb8", tolerance_km: 100, items: strip(places(rows).filter((p) => !/\b(project|power station)\b/i.test(p.name)).slice(0, 45)) };
  },
  capitals: async () => {
    // The capitals are public facts, listed by hand (Wikidata also holds winter and legislative capitals, which would be
    // asked as if they were the capital). Each position is checked against Wikidata's and a difference over 0.25 degrees is
    // reported, so a typo here shows up when the script runs.
    const LIST = [
      ["Amaravati", 16.51, 80.52, "Andhra Pradesh"], ["Itanagar", 27.08, 93.61, "Arunachal Pradesh"], ["Dispur", 26.14, 91.79, "Assam"], ["Patna", 25.6, 85.12, "Bihar"], ["Raipur", 21.25, 81.63, "Chhattisgarh"], ["Panaji", 15.5, 73.83, "Goa"],
      ["Gandhinagar", 23.22, 72.65, "Gujarat"], ["Shimla", 31.1, 77.17, "Himachal Pradesh"], ["Ranchi", 23.34, 85.31, "Jharkhand"], ["Bengaluru", 12.97, 77.59, "Karnataka"], ["Thiruvananthapuram", 8.52, 76.94, "Kerala"], ["Bhopal", 23.26, 77.41, "Madhya Pradesh"],
      ["Mumbai", 19.08, 72.88, "Maharashtra"], ["Imphal", 24.82, 93.94, "Manipur"], ["Shillong", 25.58, 91.89, "Meghalaya"], ["Aizawl", 23.73, 92.72, "Mizoram"], ["Kohima", 25.67, 94.11, "Nagaland"], ["Bhubaneswar", 20.3, 85.82, "Odisha"],
      ["Chandigarh", 30.73, 76.78, "Punjab, Haryana and the Union Territory of Chandigarh"], ["Jaipur", 26.91, 75.79, "Rajasthan"], ["Gangtok", 27.34, 88.61, "Sikkim"], ["Chennai", 13.08, 80.27, "Tamil Nadu"], ["Hyderabad", 17.38, 78.48, "Telangana"],
      ["Agartala", 23.83, 91.28, "Tripura"], ["Lucknow", 26.85, 80.95, "Uttar Pradesh"], ["Dehradun", 30.32, 78.03, "Uttarakhand"], ["Kolkata", 22.57, 88.36, "West Bengal"],
      ["Port Blair (Sri Vijaya Puram)", 11.62, 92.73, "Andaman and Nicobar Islands"], ["Daman", 20.4, 72.83, "Dadra and Nagar Haveli and Daman and Diu"], ["New Delhi", 28.61, 77.21, "Delhi"], ["Srinagar", 34.08, 74.8, "Jammu and Kashmir (summer; Jammu in winter)"],
      ["Leh", 34.16, 77.58, "Ladakh"], ["Kavaratti", 10.57, 72.64, "Lakshadweep"], ["Puducherry", 11.93, 79.83, "Puducherry"],
    ];
    const wd = await sparql("SELECT ?x ?xLabel ?c WHERE { wd:Q668 wdt:P150 ?s . ?s wdt:P36 ?x . ?x wdt:P625 ?c . SERVICE wikibase:label { bd:serviceParam wikibase:language \"en\" . } }", "capitals-check");
    for (const [name, lat, lon] of LIST) {
      const r = wd.find((x) => x.xLabel?.value === name.split(" (")[0]);
      const at = r && wkt(r.c.value);
      if (at && (Math.abs(at.lat - lat) > 0.25 || Math.abs(at.lon - lon) > 0.25)) console.warn(`capitals: ${name} is ${lat},${lon} here and ${at.lat},${at.lon} on Wikidata`);
    }
    const items = LIST.map(([name, lat, lon, of]) => ({ id: slug(name.split(" (")[0]), name, lon, lat, state: stateOf(lon, lat) ?? of.split(",")[0], detail: `Capital of ${of}` }));
    return { id: "capitals", title: "State and union territory capitals", category: "political", blurb: "The capital of every state and union territory.", kind: "point", base: "india", approx: true, source: "Public facts (the capitals), listed by hand; positions checked against Wikidata", licence: "Facts, no copyright; positions checked against Wikidata (CC0)", asOf: today, color: "#a0245a", tolerance_km: 80, items };
  },
  airports: async () => {
    // Wikidata lists some places as international airports that are not (yet) open as one, or are a military field.
    const NOT_YET = new Set(["hal-airport", "kalyan-international-airport", "rajkot-international-airport", "noida-international-airport", "alluri-sitarama-raju-international-airport"]);
    const rows = await sparql(q(`?x wdt:P31 wd:Q644371 ; wdt:P17 ${IND} .`, "", "?sl", 120), "airports");
    return { ...WD, id: "airports", title: "International airports", category: "economy", blurb: "India's international airports.", color: "#4a4a8a", tolerance_km: 80, items: strip(places(rows).filter((p) => !NOT_YET.has(p.id)).slice(0, 30)) };
  },
  cities: async () => {
    const rows = await sparql(q(`?x wdt:P31/wdt:P279* wd:Q515 ; wdt:P17 ${IND} ; wdt:P1082 ?pop . FILTER(?pop >= 1000000)`, "?pop", "?pop", 300), "cities");
    const list = places(rows, { detail: (v) => (v.pop ? `${(v.pop / 1e6).toFixed(1)} million (latest figure on Wikidata)` : "") });
    return { ...WD, id: "cities", title: "Million-plus cities", category: "economy", blurb: "Cities with a population of a million or more.", color: "#6a6a6a", tolerance_km: 80, items: strip(list.sort((a, b) => b._v.pop - a._v.pop).slice(0, 55)) };
  },
  heritage: async () => {
    const rows = await sparql(`SELECT ?x ?xLabel ?c ?sl ?year WHERE { ?x p:P1435 ?st . ?st ps:P1435 wd:Q9259 . OPTIONAL { ?st pq:P580 ?d . BIND(YEAR(?d) AS ?year) } ?x wdt:P17 ${IND} ; wdt:P625 ?c ; wikibase:sitelinks ?sl . SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . } } ORDER BY DESC(?sl) LIMIT 100`, "heritage");
    const list = places(rows, { detail: (v) => (v.year ? `UNESCO World Heritage Site since ${v.year}` : "UNESCO World Heritage Site") });
    return { ...WD, id: "heritage", title: "UNESCO World Heritage Sites", category: "heritage", blurb: "India's World Heritage Sites with the year each was inscribed.", color: "#b8741d", tolerance_km: 100, items: strip(list.filter((p) => !["iron-pillar-of-delhi", "qutb-complex"].includes(p.id)).sort((a, b) => b._sl - a._sl)) };
  },
};

// Major power plants, coloured by fuel (WRI Global Power Plant Database v1.3, CC BY 4.0). Coal, gas and hydro are cut off
// by size, and nuclear, solar and wind much lower, so that every kind has enough places to learn. Names in the database are
// capitalised and abbreviated oddly, so they are tidied and a few are corrected by hand below.
const MIN_MW = { coal: 2000, gas: 1000, hydro: 700, nuclear: 400, solar: 300, wind: 300, oil: 99999 };
// Keyed by the database's name with everything but letters and digits removed and lower-cased.
const RENAME = {
  vindhchalstps: "Vindhyachal STPS", rgundemstps: "Ramagundam STPS", kgudemnew: "Kothagudem TPS (new)", koynacomplex: "Koyna", nathpajhakri: "Nathpa Jhakri", pryagrajbaratpp: "Prayagraj (Bara) TPP",
  shrisingajimalwatpp: "Shri Singaji TPP", indragandhistpp: "Indira Gandhi STPP", chandrapurcoal: "Chandrapur STPS", vallurntpcntecl: "Vallur TPP", nasiktpp: "Nashik TPP", nasiktppph1: "Nashik TPP", kundankulam: "Kudankulam", neyvelist2: "Neyveli TPS II",
  pragaticccpiii: "Pragati CCPP-III", dgenmegacccp: "Dahej (DGEN) CCPP", sugencccp: "Sugen CCPP", ssarovarrbph: "Sardar Sarovar", teestaiii: "Teesta III", bhsp3: "Bhadla Solar Park", kumspgreenko: "Kurnool Ultra Mega Solar Park",
  npkspgreenko: "NP Kunta Solar Park", paspprenew2: "Pavagada Solar Park", pasprenew2: "Pavagada Solar Park", raps: "Rawatbhata (RAPS)", rapsx: "Rawatbhata (RAPS)", mapp: "Kalpakkam (MAPS)", naps: "Narora (NAPS)", tehrist1: "Tehri",
  puruliapss: "Purulia Pumped Storage", srisailamlbph: "Srisailam", kamuthisolarpowerplant: "Kamuthi Solar Power Project", kakrapara: "Kakrapar", sharavathy: "Sharavathi", kalinadi: "Kali River (Kalinadi)",
};
const norm = (n) => n.toLowerCase().replace(/[^a-z0-9]/g, "");
const tidy = (n) => RENAME[norm(n)] ?? (n === n.toUpperCase() || n.includes("_") ? n.replace(/_/g, " ").toLowerCase().replace(/\b([a-z])/g, (m) => m.toUpperCase()).replace(/\b(Stps|Tpp|Tps|Stpp|Ccpp|Umpp|Gt)\b/g, (m) => m.toUpperCase()).replace(/\s+/g, " ").trim() : n.trim());
defs.power = async () => {
  const csv = await get("https://raw.githubusercontent.com/wri/global-power-plant-database/master/output_database/global_power_plant_database.csv", "gppd.csv");
  const lines = csv.split(/\r?\n/);
  const head = lines[0].split(",");
  const col = (n) => head.indexOf(n);
  const cell = (l) => {
    const out = [];
    let cur = "";
    let quoted = false;
    for (const ch of l) {
      if (ch === '"') quoted = !quoted;
      else if (ch === "," && !quoted) { out.push(cur); cur = ""; } else cur += ch;
    }
    out.push(cur);
    return out;
  };
  const FUEL = { Coal: ["coal", "Coal", "#3a3a3a"], Hydro: ["hydro", "Hydro", "#1d5fb8"], Nuclear: ["nuclear", "Nuclear", "#c4a000"], Gas: ["gas", "Gas", "#d1531a"], Solar: ["solar", "Solar", "#e3a008"], Wind: ["wind", "Wind", "#2f7d5b"] };
  const items = [];
  for (const l of lines.slice(1).filter((x) => x.startsWith("IND,"))) {
    const c = cell(l);
    const mw = Number(c[col("capacity_mw")]);
    const lat = Number(c[col("latitude")]);
    const lon = Number(c[col("longitude")]);
    const fuel = FUEL[c[col("primary_fuel")]];
    if (!fuel || !Number.isFinite(lat) || !(mw >= MIN_MW[fuel[0]])) continue;
    const state = stateOf(lon, lat);
    if (!state) continue;
    items.push({ name: tidy(c[col("name")]), lon, lat, mw, fuel, state });
  }
  items.sort((a, b) => b.mw - a.mw);
  // One station listed twice (units under two names, or two spellings): keep the larger when the same fuel is within 0.3 degrees.
  const kept = [];
  for (const i of items) if (!kept.some((k) => k.fuel === i.fuel && Math.hypot(k.lon - i.lon, k.lat - i.lat) < 0.3)) kept.push(i);
  const seen = new Set();
  const out = kept.filter((i) => !seen.has(slug(i.name)) && seen.add(slug(i.name))).map((i) => ({ id: slug(i.name), name: i.name, lon: round(i.lon), lat: round(i.lat), state: i.state, detail: `${i.fuel[1]} · ${Math.round(i.mw).toLocaleString("en")} MW`, type: i.fuel[0] }));
  const used = new Set(out.map((i) => i.type));
  return {
    id: "power", title: "Major power plants", category: "economy", blurb: "The big coal, hydro, nuclear, gas, solar and wind stations, coloured by fuel.", kind: "point", base: "india",
    note: "Plants of at least 2,000 MW (coal), 1,000 MW (gas), 700 MW (hydro), 400 MW (nuclear) and 300 MW (solar and wind).",
    source: "WRI Global Power Plant Database (v1.3), India; names tidied", licence: "CC BY 4.0 (World Resources Institute, Global Power Plant Database)", asOf: today, color: "#3a3a3a", tolerance_km: 100,
    types: Object.values(FUEL).filter((f) => used.has(f[0])).map(([id, name, color]) => ({ id, name, color, also: "" })), items: out,
  };
};

// River courses: Natural Earth lines for the rivers it names, clipped to the map and thinned; the Godavari and the Mahanadi
// are not named there, so their courses are drawn from public waypoints and marked approximate.
const RIVERS = [
  ["Ganges", "Ganga", "himalayan"], ["Yamuna", "Yamuna", "himalayan"], ["Brahmaputra", "Brahmaputra", "himalayan"], ["Indus", "Indus", "himalayan"], ["Sutlej", "Sutlej", "himalayan"], ["Beas", "Beas", "himalayan"],
  ["Ravi", "Ravi", "himalayan"], ["Chenab", "Chenab", "himalayan"], ["Jhelum", "Jhelum", "himalayan"], ["Gandak", "Gandak", "himalayan"],
  ["Krishna", "Krishna", "peninsular"], ["Cauvery", "Kaveri", "peninsular"], ["Narmada", "Narmada", "peninsular"], ["Tapi", "Tapti", "peninsular"], ["Chambal", "Chambal", "peninsular"], ["Son", "Son", "peninsular"], ["Betwa", "Betwa", "peninsular"],
  ["Bhima", "Bhima", "peninsular"], ["Tungabhadra", "Tungabhadra", "peninsular"], ["Mahi", "Mahi", "peninsular"], ["Penner", "Pennar", "peninsular"], ["Brahmani", "Brahmani", "peninsular"], ["Wainganga", "Wainganga", "peninsular"],
];
const WAYPOINTS = {
  Godavari: [[73.53, 19.93], [74.5, 19.9], [75.9, 19.5], [77.32, 19.15], [77.99, 18.88], [79.0, 18.78], [79.45, 18.76], [80.3, 18.4], [80.88, 17.67], [81.5, 17.2], [81.78, 17.0], [82.3, 16.7]],
  Mahanadi: [[81.9, 20.3], [82.2, 20.7], [83.0, 21.2], [83.97, 21.47], [84.7, 21.1], [85.3, 20.6], [85.88, 20.46], [86.3, 20.35], [86.7, 20.3]],
};
const B = { minLon: 68, maxLon: 97.5, minLat: 6.5, maxLat: 37.2 };
function thin(pts, eps) {
  if (pts.length < 3) return pts;
  const [a, b] = [pts[0], pts[pts.length - 1]];
  let worst = 0;
  let at = 0;
  for (let i = 1; i < pts.length - 1; i++) {
    const d = Math.abs((b[1] - a[1]) * pts[i][0] - (b[0] - a[0]) * pts[i][1] + b[0] * a[1] - b[1] * a[0]) / (Math.hypot(b[0] - a[0], b[1] - a[1]) || 1);
    if (d > worst) { worst = d; at = i; }
  }
  return worst > eps ? [...thin(pts.slice(0, at + 1), eps).slice(0, -1), ...thin(pts.slice(at), eps)] : [a, b];
}
defs.rivers_lines = async () => {
  const geo = JSON.parse(await get("https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_10m_rivers_lake_centerlines.geojson", "rivers.geojson"));
  const lines = [];
  for (const [ne, name, type] of RIVERS) {
    const parts = geo.features.filter((f) => (f.properties.name ?? "") === ne).flatMap((f) => (f.geometry.type === "MultiLineString" ? f.geometry.coordinates : [f.geometry.coordinates]));
    const segs = parts.map((p) => p.filter(([lon, lat]) => lon >= B.minLon && lon <= B.maxLon && lat >= B.minLat && lat <= B.maxLat)).filter((p) => p.length > 1);
    if (!segs.length) { console.warn(`no line for ${ne}`); continue; }
    const d = segs.map((s) => "M" + thin(s.map(([lon, lat]) => proj(lon, lat)), 0.5).map(([x, y]) => `${round(x, 1)} ${round(y, 1)}`).join("L")).join("");
    lines.push({ id: slug(name), name, d, type });
  }
  for (const [name, pts] of Object.entries(WAYPOINTS)) {
    lines.push({ id: slug(name), name, d: "M" + pts.map(([lon, lat]) => proj(lon, lat)).map(([x, y]) => `${round(x, 1)} ${round(y, 1)}`).join("L"), type: "peninsular" });
  }
  lines.sort((a, b) => a.name.localeCompare(b.name));
  return {
    id: "river-systems", title: "River systems", category: "rivers", blurb: "The Himalayan and the peninsular rivers: where each flows.", kind: "line", base: "india", approx: true,
    note: "The Godavari and the Mahanadi are drawn from a few waypoints, so their courses are rough. The other rivers follow Natural Earth lines, cut off at the edge of the map.",
    source: "Natural Earth rivers (ne_10m_rivers_lake_centerlines); the Godavari and Mahanadi drawn from public waypoints", licence: "Public domain (Natural Earth)", asOf: today, color: "#1d5fb8", tolerance_km: 60,
    types: [{ id: "himalayan", name: "Himalayan rivers", color: "#1d5fb8", also: "Fed by snow and rain: perennial. The Indus, Ganga and Brahmaputra systems." }, { id: "peninsular", name: "Peninsular rivers", color: "#2f7d5b", also: "Fed by rain: seasonal, mostly flowing east into the Bay of Bengal, the Narmada and Tapti west." }],
    lines,
  };
};

// ---- run --------------------------------------------------------------------------------------------------------------------------
const want = process.argv.slice(2);
const dump = (m) => writeFileSync(new URL(`${m.id}.json`, OUT), JSON.stringify(Object.fromEntries(Object.entries({ id: m.id, title: m.title, category: m.category, blurb: m.blurb, kind: m.kind ?? "point", base: "india", source: m.source, licence: m.licence, asOf: m.asOf, note: m.note, approx: m.approx, color: m.color, tolerance_km: m.tolerance_km, items: m.items, types: m.types, lines: m.lines }).filter(([, v]) => v !== undefined))));
for (const [key, build] of Object.entries(defs)) {
  if (want.length && !want.includes(key)) continue;
  try {
    const m = await build();
    const n = m.items?.length ?? m.lines?.length ?? 0;
    if (n < MIN_ITEMS) { console.warn(`${m.id}: only ${n} items, not written`); continue; }
    dump(m);
    console.log(`${m.id}: ${n}`);
    console.log("   " + (m.items ?? m.lines).slice(0, 60).map((i) => `${i.name}${i.state ? ` (${i.state})` : ""}`).join("; "));
  } catch (e) {
    console.warn(`${key}: ${e.message}`);
  }
}
