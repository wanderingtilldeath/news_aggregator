// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { KINDS, countByKind, filterMaps, kindLabel } from "./map-catalog.ts";
import type { MapSummary } from "./maps.ts";

const m = (id: string, title: string, category: string, blurb = ""): MapSummary => ({ id, title, category, blurb, kind: "point", base: "india", source: "s", licence: "l", asOf: "2026-11", count: 10 });
const maps = [m("a", "Tiger reserves", "wildlife", "The 58 tiger reserves"), m("b", "National parks", "wildlife", "Parks of India"), m("c", "Major ports", "economy", "Where ships call"), m("d", "Soils", "climate", "Which soil covers which state")];

// Counts per kind, in the picker's order, leaving out the empty kinds.
assert.deepEqual(countByKind(maps).map((k) => [k.id, k.count]), [["climate", 1], ["wildlife", 2], ["economy", 1]]);
assert.equal(KINDS.length, new Set(KINDS.map((k) => k.id)).size);
assert.equal(kindLabel("rivers"), "Rivers and dams");
assert.equal(kindLabel("nope"), "nope");

// Filter by kind and by words; every word has to match somewhere in the title, the blurb or the kind's label.
assert.deepEqual(filterMaps(maps, "", "").map((x) => x.id), ["a", "b", "c", "d"]);
assert.deepEqual(filterMaps(maps, "wildlife", "").map((x) => x.id), ["a", "b"]);
assert.deepEqual(filterMaps(maps, "", "TIGER").map((x) => x.id), ["a"]);
assert.deepEqual(filterMaps(maps, "", "soil state").map((x) => x.id), ["d"]);
assert.deepEqual(filterMaps(maps, "", "protected").map((x) => x.id), ["a", "b"], "the kind's label counts");
assert.deepEqual(filterMaps(maps, "wildlife", "ports"), []);
assert.deepEqual(filterMaps(maps, "", "  "), maps);

// scripts/map-index.mjs orders the kinds the same way.
const script = readFileSync(new URL("../scripts/map-index.mjs", import.meta.url), "utf8");
const fromScript = [...(/const KINDS = \[(.*?)\];/.exec(script)![1].matchAll(/"([a-z-]+)"/g))].map((x) => x[1]);
assert.deepEqual(fromScript, KINDS.map((k) => k.id));

// The generated index agrees with the maps themselves: one entry per file, every kind known.
const index = JSON.parse(readFileSync(new URL("./data/maps/index.json", import.meta.url), "utf8")) as MapSummary[];
assert.ok(index.length >= 11);
assert.ok(index.every((e) => KINDS.some((k) => k.id === e.category) && e.count > 0), "every map has a known kind and something on it");

console.log("map-catalog: ok");
