// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { readdirSync } from "node:fs";
import { COMBOS, KINDS, MAX_COMBINED, combinable, combine, countByKind, filterMaps, kindLabel } from "./map-catalog.ts";
import { questionsFor } from "./map-test.ts";
import type { MapLayer, MapSummary } from "./maps.ts";

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

// ---- combined maps ----
const all = Object.fromEntries(
  readdirSync(new URL("./data/maps/", import.meta.url))
    .filter((f) => f.endsWith(".json") && f !== "index.json")
    .map((f) => JSON.parse(readFileSync(new URL(`./data/maps/${f}`, import.meta.url), "utf8")) as MapLayer)
    .map((m) => [m.id, m]),
) as Record<string, MapLayer>;
const get = (ids: readonly string[]) => ids.map((id) => all[id]);

// Every offered pair really combines, and the result keeps every place of every layer under its own colour.
for (const c of COMBOS) {
  const layers = get(c.ids);
  assert.ok(layers.every(Boolean), `${c.label}: a map in the list does not exist`);
  const r = combine(layers);
  assert.ok(r.map, `${c.label}: ${r.error}`);
  const m = r.map;
  assert.equal((m.items?.length ?? 0) + (m.lines?.length ?? 0), layers.reduce((n, l) => n + (l.items?.length ?? 0) + (l.lines?.length ?? 0), 0), `${c.label}: nothing lost`);
  assert.equal(new Set([...(m.items ?? []), ...(m.lines ?? [])].map((x) => x.id)).size, (m.items?.length ?? 0) + (m.lines?.length ?? 0), `${c.label}: ids stay unique`);
  assert.deepEqual(m.types?.map((t) => t.id), c.ids);
  assert.equal(new Set(m.types?.map((t) => t.color)).size, c.ids.length, `${c.label}: one colour a layer`);
  assert.ok((m.items ?? []).every((i) => m.types?.some((t) => t.id === i.type)));
}

// Rivers (lines) with dams (dots): a test can ask for both, and a river question says which layer it is from.
const rd = combine(get(["river-systems", "dams"])).map!;
assert.equal(rd.kind, "point");
const qs = questionsFor(rd, []);
assert.equal(qs.filter((q) => q.kind === "line").length, all["river-systems"].lines!.length);
assert.equal(qs.filter((q) => q.kind === "place").length, all.dams.items!.length);
assert.ok(qs.filter((q) => q.kind === "line").every((q) => q.hint === "River systems"));
// The same choice in any order is the same map (so a best score is kept once).
assert.equal(combine(get(["dams", "river-systems"])).map!.id, rd.id);
// What cannot be combined says why.
assert.match(combine([all.dams]).error!, /at least two/);
assert.match(combine(get(["dams", "soils"])).error!, /shaded by state/);
assert.match(combine(get(["dams", "world-seas"])).error!, /India and a map of the world/);
assert.match(combine(get(["dams", "ports", "airports", "power", "cities"])).error!, new RegExp(`at most ${MAX_COMBINED}`));
assert.ok(combinable(all.dams) && !combinable(all.soils) && !combinable(all.states));

console.log("map-catalog: ok");
