// Builds the world outline (lib/data/world.json) and the world maps of lib/data/maps (run by hand, commit the output, then
// run scripts/map-index.mjs). Needs the network.
//   node scripts/world-data.mjs
// The outline is Natural Earth 110m LAND (public domain) from the world-atlas package, as one silhouette with no country
// borders: the maps teach where places are, and drawing no borders means none can be drawn wrongly (India's are not drawn
// as the Survey of India draws them in the other outlines this site has). Places come from Wikidata (CC0).
// The projection constants must match WORLD in lib/maps.ts (lib/maps.test.ts checks it).
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const WORLD = { s: 3, top: 85, bottom: -60 }; // x = (lon + 180) * s, y = (top - lat) * s; Antarctica is left off
const OUT = new URL("../lib/data/maps/", import.meta.url);
const CACHE = join(tmpdir(), "pp-map-data");
mkdirSync(CACHE, { recursive: true });
const UA = "PostpaperMapData/1.0 (https://www.postpaper.in; one-off build of open-data revision maps)";
const today = new Date().toISOString().slice(0, 7);
const round = (n, d = 3) => Math.round(n * 10 ** d) / 10 ** d;
const slug = (s) => s.toLowerCase().normalize("NFD").replace(/[̀-ͯ]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");

async function get(url, file, init = {}) {
  const path = join(CACHE, file);
  if (existsSync(path)) return readFileSync(path, "utf8");
  let res;
  for (let attempt = 0; attempt < 4; attempt++) {
    res = await fetch(url, { ...init, headers: { "User-Agent": UA, ...(init.headers ?? {}) }, signal: AbortSignal.timeout(180_000) });
    if (![429, 502, 503, 504].includes(res.status)) break;
    await new Promise((r) => setTimeout(r, 30_000 * (attempt + 1)));
  }
  if (!res.ok) throw new Error(`${url.slice(0, 80)}…: ${res.status}`);
  const text = await res.text();
  writeFileSync(path, text);
  return text;
}
const sparql = async (query, name) => (console.log(`asking Wikidata: ${name}`), JSON.parse(await get("https://query.wikidata.org/sparql?format=json&query=" + encodeURIComponent(query), `wd-${name}.json`, { headers: { Accept: "application/sparql-results+json" } }))).results.bindings;
const wkt = (s) => {
  const m = /Point\(([-0-9.]+) ([-0-9.]+)\)/.exec(s);
  return m ? { lon: Number(m[1]), lat: Number(m[2]) } : null;
};

// ---- the outline -------------------------------------------------------------------------------------------------------------
const topo = JSON.parse(await get("https://cdn.jsdelivr.net/npm/world-atlas@2.0.2/land-110m.json", "land110.json"));
const { scale, translate } = topo.transform;
const arcs = topo.arcs.map((arc) => {
  let x = 0;
  let y = 0;
  return arc.map(([dx, dy]) => [(x += dx) * scale[0] + translate[0], (y += dy) * scale[1] + translate[1]]);
});
const ring = (idx) => idx.flatMap((i, n) => {
  const pts = i >= 0 ? arcs[i] : [...arcs[~i]].reverse();
  return n ? pts.slice(1) : pts;
});
const px = (lon, lat) => [round((lon + 180) * WORLD.s, 1), round((WORLD.top - lat) * WORLD.s, 1)];
const polys = topo.objects.land.geometries.flatMap((g) => (g.type === "Polygon" ? [g.arcs] : g.arcs));
const rings = polys.flatMap((p) => p.map(ring)).filter((r) => Math.max(...r.map((p) => p[1])) > WORLD.bottom);
// Antarctica and anything reaching below the map's bottom edge is clamped to it so it does not run off the picture.
// A ring that crosses the antimeridian (Russia's far east, Fiji) is unwrapped so it has no line across the whole map, and
// drawn again 360 degrees over so the part that left one edge shows on the other. The picture's box clips the overhang.
const unwrap = (r) => r.reduce((out, [lon, lat], n) => {
  const prev = n ? out[n - 1][0] : lon;
  out.push([lon + 360 * Math.round((prev - lon) / 360), lat]);
  return out;
}, []);
const shapes = rings.flatMap((r) => {
  const u = unwrap(r);
  const lons = u.map((p) => p[0]);
  const over = Math.max(...lons) > 180 ? -360 : Math.min(...lons) < -180 ? 360 : 0;
  return over ? [u, u.map(([lon, lat]) => [lon + over, lat])] : [u];
});
const d = shapes.map((r) => "M" + r.map(([lon, lat]) => px(lon, Math.max(lat, WORLD.bottom)).join(" ")).join("L") + "Z").join("");
const w = 360 * WORLD.s;
const h = (WORLD.top - WORLD.bottom) * WORLD.s;
writeFileSync(new URL("../lib/data/world.json", import.meta.url), JSON.stringify({ w, h, d }));
console.log(`world outline: ${w}x${h}, ${rings.length} rings, ${(d.length / 1024).toFixed(0)} KB of path`);

// ---- the maps ------------------------------------------------------------------------------------------------------------------
const WD = { kind: "point", base: "world", source: "Wikidata, queried for the places with a position and an English label", licence: "CC0 (Wikidata)", asOf: today };
const q = (body, vars = "", limit = 300) => `SELECT ?x ?xLabel ?c ?sl ${vars} WHERE { ${body} ?x wdt:P625 ?c . ?x wikibase:sitelinks ?sl . SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . } } ORDER BY DESC(?sl) LIMIT ${limit}`;
const rowsOf = (rows, { extra } = {}) => {
  const seen = new Map();
  for (const r of rows) {
    const name = r.xLabel?.value?.split(",")[0].trim();
    const at = r.c && wkt(r.c.value);
    if (!name || /^Q\d+$/.test(name) || !at || at.lat < WORLD.bottom || at.lat > WORLD.top) continue;
    const id = slug(name);
    if (!id || seen.has(id)) continue;
    seen.set(id, { id, name, lon: round(at.lon), lat: round(at.lat), sl: Number(r.sl?.value ?? 0), ...(extra ? extra(r) : {}) });
  }
  return [...seen.values()];
};
// Wikidata items that are not what the map is for (a river, a bay of a city, a peak among ranges) or that another map has.
const DROP = new Set(["east-river", "golden-gate", "st-george-s-channel", "indian-ocean", "english-channel", "kattegat", "ha-long-bay", "annapurna", "sinai-peninsula", "death-valley", "eifel", "tassili-n-ajjer", "greater-caucasus", "negev"]);
const RENAME = { "Southern Alps / Kā Tiritiri o te Moana": "Southern Alps" };
const clean = (items) => items.filter((i) => !DROP.has(i.id)).map((i) => ({ ...Object.fromEntries(Object.entries(i).filter(([k]) => k !== "sl")), name: RENAME[i.name] ?? i.name, id: RENAME[i.name] ? slug(RENAME[i.name]) : i.id }));

const maps = [];

// Capitals of the sovereign states, one per capital (a country with two, such as Bolivia, has both).
{
  // Sovereign states; wdt:P36 is the current capital. A country that no longer exists has no current capital on Wikidata.
  const rows = await sparql(`SELECT ?x ?xLabel ?c ?sLabel WHERE { ?s wdt:P31 wd:Q3624078 ; wdt:P36 ?x . ?x wdt:P625 ?c . SERVICE wikibase:label { bd:serviceParam wikibase:language "en" . } }`, "world-capitals3");
  const items = rowsOf(rows, { extra: (r) => ({ country: r.sLabel?.value }) }).filter((i) => i.country && !/^Q\d+$/.test(i.country));
  maps.push({
    ...WD, id: "world-capitals", title: "Capitals of the world", category: "world-capitals", blurb: "The capital of each country: find it on the world map.", color: "#a0245a", tolerance_km: 250,
    note: "Capitals as recorded on Wikidata. A few capitals are disputed; the map takes no side.",
    items: clean(items.sort((a, b) => a.country.localeCompare(b.country))).map(({ country, ...i }) => ({ ...i, state: "", detail: `Capital of ${country}`, ask: `Find the capital of ${country}` })),
  });
}
// Straits, then seas, gulfs and bays.
{
  const rows = await sparql(q("?x wdt:P31 wd:Q37901 .", "", 200), "world-straits");
  maps.push({ ...WD, id: "world-straits", title: "Straits", category: "world-seas", blurb: "Hormuz, Malacca, Bab-el-Mandeb, Gibraltar and the other straits that ships and news pass through.", color: "#1d5fb8", tolerance_km: 350, approx: true, items: clean(rowsOf(rows).slice(0, 44)).map((i) => ({ ...i, state: "" })).slice(0, 38) });
}
{
  const rows = await sparql(q("VALUES ?k { wd:Q165 wd:Q39594 wd:Q1322134 } ?x wdt:P31 ?k .", "", 300), "world-seas2");
  maps.push({ ...WD, id: "world-seas", title: "Seas, gulfs and bays", category: "world-seas", blurb: "The seas, gulfs and bays of the world. Dots are approximate centres.", color: "#2f6fbf", tolerance_km: 700, approx: true, items: clean(rowsOf(rows).slice(0, 52)).map((i) => ({ ...i, state: "" })).slice(0, 45) });
}
// Mountain ranges and deserts together, told apart by colour.
{
  const ranges = rowsOf(await sparql(q("?x wdt:P31 wd:Q46831 .", "", 200), "world-ranges"), { extra: () => ({ type: "mountain" }) }).slice(0, 38);
  const deserts = rowsOf(await sparql(q("?x wdt:P31 wd:Q8514 .", "", 120), "world-deserts"), { extra: () => ({ type: "desert" }) }).slice(0, 24);
  maps.push({
    ...WD, id: "world-land", title: "Mountains and deserts", category: "world-land", blurb: "The great mountain ranges and deserts of the world. Dots are approximate centres.", color: "#8a5a2a", tolerance_km: 700, approx: true,
    types: [{ id: "mountain", name: "Mountain ranges", color: "#7a4a9a", also: "" }, { id: "desert", name: "Deserts", color: "#c4943a", also: "" }],
    items: clean([...ranges, ...deserts]).map((i) => ({ ...i, state: "" })),
  });
}

for (const m of maps) {
  if (m.items.length < 8) { console.warn(`${m.id}: only ${m.items.length} items, not written`); continue; }
  const out = Object.fromEntries(Object.entries({ id: m.id, title: m.title, category: m.category, blurb: m.blurb, kind: m.kind, base: m.base, source: m.source, licence: m.licence, asOf: m.asOf, note: m.note, approx: m.approx, color: m.color, tolerance_km: m.tolerance_km, items: m.items, types: m.types }).filter(([, v]) => v !== undefined));
  writeFileSync(new URL(`${m.id}.json`, OUT), JSON.stringify(out));
  console.log(`${m.id}: ${m.items.length}`);
  console.log("   " + m.items.slice(0, 70).map((i) => i.name).join("; "));
}
