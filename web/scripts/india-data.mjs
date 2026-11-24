// Builds lib/data/india.json, the state shapes of the India map (run once, commit the output; the page reads the JSON).
//   mkdir geo && curl -L -o geo/india.topo.json https://cdn.jsdelivr.net/gh/udit-001/india-maps-data@2884453/topojson/india.json
//   node scripts/india-data.mjs geo/india.topo.json
// Input: the "states" object of that TopoJSON (36 states and union territories, 2011 census codes). The file is
// curated from public sources and its licence is not stated: the owner must check the outline against the Survey of
// India one (Jammu and Kashmir, Ladakh, Arunachal Pradesh) before release. No dependency: arcs are decoded here.
// The projection constants must match PROJ in lib/maps.ts (lib/maps.test.ts checks it).
import { readFileSync, writeFileSync } from "node:fs";

const PROJ = { lon0: 68, lat1: 37.2, k: 0.92, s: 36 }; // x = (lon - lon0) * k * s, y = (lat1 - lat) * s
const topo = JSON.parse(readFileSync(process.argv[2], "utf8"));
const { scale, translate } = topo.transform;

// Decode the delta-encoded arcs into lon/lat points.
const arcs = topo.arcs.map((arc) => {
  let x = 0, y = 0;
  return arc.map(([dx, dy]) => [(x += dx) * scale[0] + translate[0], (y += dy) * scale[1] + translate[1]]);
});
const ring = (idx) => idx.flatMap((i, n) => {
  const pts = i >= 0 ? arcs[i] : [...arcs[~i]].reverse();
  return n ? pts.slice(1) : pts;
});
const r1 = (n) => Math.round(n * 10) / 10;
const draw = (pts) => pts.map(([lon, lat], n) => `${n ? "L" : "M"}${r1((lon - PROJ.lon0) * PROJ.k * PROJ.s)} ${r1((PROJ.lat1 - lat) * PROJ.s)}`).join("") + "Z";

const states = topo.objects.states.geometries.map((g) => {
  const polys = g.type === "Polygon" ? [g.arcs] : g.arcs;
  const d = polys.flatMap((p) => p.map((r) => draw(ring(r)))).join("");
  return { id: g.properties.st_nm.toLowerCase().replace(/[^a-z]+/g, "-").replace(/^-|-$/g, ""), name: g.properties.st_nm, d };
}).sort((a, b) => a.name.localeCompare(b.name));

const [minLon, minLat, maxLon, maxLat] = topo.bbox;
const out = { w: Math.ceil((maxLon - PROJ.lon0) * PROJ.k * PROJ.s), h: Math.ceil((PROJ.lat1 - minLat) * PROJ.s), states };
writeFileSync(new URL("../lib/data/india.json", import.meta.url), JSON.stringify(out));
console.log(`${states.length} states, ${out.w}x${out.h}, bbox ${minLon.toFixed(1)},${minLat.toFixed(1)} to ${maxLon.toFixed(1)},${maxLat.toFixed(1)}`);
