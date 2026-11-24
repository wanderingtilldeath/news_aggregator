// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { MAX_LABEL, MAX_MARKERS, MAX_NOTE, PROJ, addMarker, inIndia, parseMarkers, project, removeMarker, unproject, updateMarker, type MapLayer } from "./maps.ts";

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

// The data: 36 states, and every ready-made layer is complete and sits inside India.
const india = JSON.parse(read("./data/india.json")) as { w: number; h: number; states: { id: string; name: string; d: string }[] };
const layers = (JSON.parse(read("./data/map-layers.json")) as { layers: MapLayer[] }).layers;
assert.equal(india.states.length, 36);
assert.ok(india.states.every((s) => s.d.startsWith("M") && s.d.endsWith("Z")));
assert.equal(new Set(layers.map((l) => l.id)).size, layers.length);
for (const l of layers) {
  assert.ok(l.source && l.asOf && l.title && l.group, l.id);
  if (l.kind === "region") {
    const types = new Set(l.types!.map((t) => t.id));
    assert.deepEqual(Object.keys(l.regions!).sort(), india.states.map((s) => s.id).sort(), `${l.id} covers every state`);
    for (const t of Object.values(l.regions!)) assert.ok(types.has(t), `${l.id}: unknown type ${t}`);
  } else {
    assert.ok(l.items!.length > 0 && l.color, l.id);
    assert.equal(new Set(l.items!.map((i) => i.id)).size, l.items!.length, `${l.id} ids are unique`);
    for (const i of l.items!) {
      assert.ok(inIndia(i.lon, i.lat), `${l.id}/${i.id} is outside India`);
      assert.ok(i.name && i.state, `${l.id}/${i.id} has no name or state`);
    }
  }
}
const count = (id: string) => layers.find((l) => l.id === id)!.items!.length;
assert.equal(count("tiger"), 58); // NTCA, as of 2025
assert.equal(count("biosphere"), 18);

console.log("maps: all checks passed");
