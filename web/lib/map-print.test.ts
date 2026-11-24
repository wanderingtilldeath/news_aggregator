// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { centre, printPoints, startOf } from "./map-print.ts";
import { project, projectWorld, type MapLayer } from "./maps.ts";

const read = (f: string) => JSON.parse(readFileSync(new URL(f, import.meta.url), "utf8"));
const india = read("./data/india.json");

// A square of 10 units: its middle is 5, 5.
assert.deepEqual(centre("M0 0L10 0L10 10L0 10Z"), { x: 5, y: 5 });

// The biggest piece wins: a small island far away does not pull the number off the mainland.
assert.deepEqual(centre("M0 0L10 0L10 10L0 10ZM100 100L102 100L102 102L100 102Z"), { x: 5, y: 5 });
assert.equal(centre(""), null);

assert.deepEqual(startOf("M271.6 191.3L267.8 188.8"), { x: 271.6, y: 191.3 });
assert.equal(startOf(""), null);

const maps: MapLayer[] = readdirSync(new URL("./data/maps/", import.meta.url))
  .filter((f) => f.endsWith(".json") && f !== "index.json")
  .map((f) => read(`./data/maps/${f}`));
assert.ok(maps.length >= 29);
const baseOf = (m: MapLayer) => (m.base === "india" ? { project, shapes: india.states } : { project: projectWorld, shapes: [{ id: "land", name: "Land", d: "M0 0L1 0L1 1Z" }] });

// Every ready-made map gives one numbered point per item, line or state, numbered from 1 without gaps.
for (const map of maps) {
  const base = baseOf(map);
  const pts = printPoints(map, base);
  const expected = map.kind === "point" ? (map.items?.length ?? 0) : map.kind === "line" ? (map.lines?.length ?? 0) : base.shapes.length;
  assert.equal(pts.length, expected, `${map.id}: one point each`);
  assert.deepEqual(pts.map((p) => p.n), pts.map((_, i) => i + 1), `${map.id}: numbered from 1`);
  assert.ok(pts.every((p) => Number.isFinite(p.x) && Number.isFinite(p.y) && p.name), `${map.id}: every point has a place and a name`);
}

// A state's number sits inside the map.
const states = maps.find((m) => m.id === "states");
assert.ok(states);
assert.ok(printPoints(states, baseOf(states)).every((p) => p.x >= 0 && p.x <= india.w && p.y >= 0 && p.y <= india.h));

// A region map puts the type of each state in the key.
const soils = maps.find((m) => m.id === "soils");
assert.ok(soils);
assert.ok(printPoints(soils, baseOf(soils)).some((p) => p.detail));

console.log("map-print: ok");
