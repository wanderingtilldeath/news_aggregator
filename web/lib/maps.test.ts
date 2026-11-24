// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { MAX_LABEL, MAX_MARKERS, MAX_NOTE, PROJ, WORLD, addMarker, inIndia, inWorld, parseMarkers, project, projectWorld, removeMarker, unproject, unprojectWorld, updateMarker, type MapLayer } from "./maps.ts";

const read = (f: string) => readFileSync(new URL(f, import.meta.url), "utf8");
const m = (id: string, lon = 77.2, lat = 28.6) => ({ id, lon, lat, label: "Delhi", note: "" });

// Parsing: bad text, wrong shapes and places outside the map are dropped; long text is cut.
assert.deepEqual(parseMarkers(""), []);
assert.deepEqual(parseMarkers("not json"), []);
assert.deepEqual(parseMarkers('{"a":1}'), []);
const parsed = parseMarkers(JSON.stringify([m("a"), { id: "b", lon: 2, lat: 2 }, { id: "c", lon: "x", lat: 1 }, null, { ...m("d"), label: "x".repeat(200), note: "y".repeat(900) }]));
assert.deepEqual(parsed.map((x) => x.id), ["a", "d"]);
assert.equal(parsed[1].label.length, MAX_LABEL);
assert.equal(parsed[1].note.length, MAX_NOTE);

// Add, update, remove.
let list = addMarker([], m("a"));
list = addMarker(list, m("b", 80, 13));
assert.deepEqual(list.map((x) => x.id), ["b", "a"]); // newest first
assert.equal(addMarker(list, m("a")), list); // the same id twice does nothing
assert.equal(addMarker(list, m("z", 0, 0)), list); // outside the map
assert.equal(addMarker(Array.from({ length: MAX_MARKERS }, (_, i) => m(String(i))), m("late")).length, MAX_MARKERS);
list = updateMarker(list, "a", { note: "capital  " });
assert.equal(list.find((x) => x.id === "a")?.note, "capital");
assert.equal(list.find((x) => x.id === "a")?.label, "Delhi"); // an untouched field stays
assert.deepEqual(removeMarker(list, "a").map((x) => x.id), ["b"]);

// Projection round trip, and the corners of India stay inside the page the generator draws.
for (const [lon, lat] of [[77.2, 28.6], [68.2, 23.5], [97.3, 28.1], [77.5, 8.1]]) {
  const p = unproject(project(lon, lat).x, project(lon, lat).y);
  assert.ok(Math.abs(p.lon - lon) < 1e-9 && Math.abs(p.lat - lat) < 1e-9);
}
assert.ok(inIndia(77.2, 28.6) && !inIndia(0, 0));

// The generator script and this file share one projection.
const script = read("../scripts/india-data.mjs");
const fromScript = /const PROJ = \{ lon0: ([\d.]+), lat1: ([\d.]+), k: ([\d.]+), s: ([\d.]+) \}/.exec(script)!.slice(1).map(Number);
assert.deepEqual(fromScript, [PROJ.lon0, PROJ.lat1, PROJ.k, PROJ.s]);

// The world projection: the same constants in scripts/world-data.mjs, a round trip, and its bounds.
const worldScript = read("../scripts/world-data.mjs");
const wm = /const WORLD = \{ s: (\d+), top: (-?\d+), bottom: (-?\d+) \}/.exec(worldScript)!;
assert.deepEqual([Number(wm[1]), Number(wm[2]), Number(wm[3])], [WORLD.s, WORLD.top, WORLD.bottom]);
for (const [lon, lat] of [[0, 0], [77.2, 28.6], [-122.4, 37.8], [151.2, -33.9]]) {
  const p = projectWorld(lon, lat);
  const q = unprojectWorld(p.x, p.y);
  assert.ok(Math.abs(q.lon - lon) < 1e-9 && Math.abs(q.lat - lat) < 1e-9);
}
assert.ok(inWorld(0, 0) && inWorld(-179, 84) && !inWorld(0, -75) && !inWorld(0, 89) && !inWorld(181, 0));
const worldOutline = JSON.parse(read("./data/world.json")) as { w: number; h: number; d: string };
assert.deepEqual([worldOutline.w, worldOutline.h], [360 * WORLD.s, (WORLD.top - WORLD.bottom) * WORLD.s]);
assert.ok(worldOutline.d.startsWith("M") && worldOutline.d.endsWith("Z") && worldOutline.d.length > 10_000);


// The data: 36 states, and every ready-made map is complete, licensed and sits inside its outline.
import { readdirSync } from "node:fs";
import { KINDS } from "./map-catalog.ts";
const india = JSON.parse(read("./data/india.json")) as { w: number; h: number; states: { id: string; name: string; d: string }[] };
const dir = new URL("./data/maps/", import.meta.url);
const files = readdirSync(dir).filter((f) => f.endsWith(".json") && f !== "index.json");
const layers = files.map((f) => JSON.parse(readFileSync(new URL(f, dir), "utf8")) as MapLayer);
assert.equal(india.states.length, 36);
assert.ok(india.states.every((s) => s.d.startsWith("M") && s.d.endsWith("Z")));
assert.equal(new Set(layers.map((l) => l.id)).size, layers.length);
assert.deepEqual(files.sort(), layers.map((l) => `${l.id}.json`).sort(), "each file is named after its map");
const MIN_ITEMS = 8; // a ten-question test needs about this many places
for (const l of layers) {
  assert.ok(l.source && l.licence && l.asOf && l.title && l.blurb, `${l.id} lacks a title, blurb, source, licence or date`);
  assert.ok(KINDS.some((k) => k.id === l.category), `${l.id}: unknown category ${l.category}`);
  assert.ok(["india", "world"].includes(l.base), `${l.id}: unknown base`);
  if (l.kind === "states") continue;
  if (l.kind === "region") {
    const types = new Set(l.types!.map((t) => t.id));
    assert.deepEqual(Object.keys(l.regions!).sort(), india.states.map((s) => s.id).sort(), `${l.id} covers every state`);
    for (const t of Object.values(l.regions!)) assert.ok(types.has(t), `${l.id}: unknown type ${t}`);
  } else if (l.kind === "line") {
    assert.ok(l.lines!.length >= MIN_ITEMS && l.color, `${l.id}: too few lines or no colour`);
    assert.equal(new Set(l.lines!.map((x) => x.id)).size, l.lines!.length, `${l.id} ids are unique`);
    assert.ok(l.lines!.every((x) => x.name && /^M[^]*[0-9]$/.test(x.d)), `${l.id}: a line has no name or no path`);
  } else {
    assert.ok(l.items!.length >= (l.id === "monsoon" ? 5 : MIN_ITEMS) && l.color, `${l.id}: too few items or no colour`);
    assert.equal(new Set(l.items!.map((i) => i.id)).size, l.items!.length, `${l.id} ids are unique`);
    for (const i of l.items!) {
      if (l.base === "india") assert.ok(inIndia(i.lon, i.lat), `${l.id}/${i.id} is outside India`);
      if (l.base === "world") assert.ok(inWorld(i.lon, i.lat), `${l.id}/${i.id} is outside the world map`);
      assert.ok(i.name && Number.isFinite(i.lon) && Number.isFinite(i.lat), `${l.id}/${i.id} has no name or place`);
      if (l.base === "india") assert.ok(i.state, `${l.id}/${i.id} has no state`);
    }
  }
}
const count = (id: string) => layers.find((l) => l.id === id)!.items!.length;
assert.equal(count("tiger"), 58); // NTCA, as of 2025
assert.equal(count("biosphere"), 18);

console.log("maps: all checks passed");
