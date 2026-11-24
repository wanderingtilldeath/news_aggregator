// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { FIRST, LAYOUT, MAX_ALL, chooseLabelled, crosses, layoutCallouts, shadedStates, splitSides, wrapLabel, type Frame, type Spot } from "./map-labels.ts";
import { labellable, printPoints } from "./map-print.ts";
import { project, type MapLayer } from "./maps.ts";

// ---- wrapping ----
assert.deepEqual(wrapLabel("Panna", 28), ["Panna"]);
assert.deepEqual(wrapLabel("Nanda Devi Biosphere Reserve", 20), ["Nanda Devi", "Biosphere Reserve"], "broken at the most even space");
assert.equal(wrapLabel("Chhatrapati Shivaji Maharaj International Airport", 28).length, 2);
assert.ok(wrapLabel("Chhatrapati Shivaji Maharaj International Airport", 28).every((l) => l.length <= 28));
assert.deepEqual(wrapLabel("Supercalifragilisticexpialidocious", 10), ["Supercali…"], "one long word is cut");
assert.ok(wrapLabel("Aaaaaaaaaaaaaaaaaaaaaaaa Bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", 12).every((l) => l.length <= 12), "never wider than the column");

// ---- what is named ----
const ten = Array.from({ length: 10 }, (_, i) => i);
assert.deepEqual(chooseLabelled(ten, "point"), { labelled: ten, keyed: [] });
const many = Array.from({ length: 58 }, (_, i) => i);
const c = chooseLabelled(many, "point");
assert.equal(c.labelled.length, FIRST);
assert.deepEqual([...c.labelled, ...c.keyed], many, "none lost, order kept");
assert.equal(chooseLabelled(Array.from({ length: MAX_ALL }, (_, i) => i), "point").keyed.length, 0, "35 are all named");
assert.equal(chooseLabelled(Array.from({ length: MAX_ALL + 1 }, (_, i) => i), "point").keyed.length, MAX_ALL + 1 - FIRST);
assert.equal(chooseLabelled(Array.from({ length: 36 }, (_, i) => i), "states").keyed.length, 0, "the states are always named in full");

// ---- sides ----
const col = (n: number, x: number): Spot[] => Array.from({ length: n }, (_, i) => ({ id: `${x}-${i}`, name: "P", x: x + i, y: i * 10 }));
const sides = splitSides([...col(10, 100), ...col(2, 900)], 1000);
assert.ok(sides.left.length <= 8 && sides.right.length <= 8, "a side holds at most 60%");
assert.equal(sides.left.length + sides.right.length, 12);

// ---- crossing ----
assert.ok(crosses([[0, 0], [10, 10]], [[0, 10], [10, 0]]));
assert.ok(!crosses([[0, 0], [10, 0]], [[10, 0], [20, 5]]), "meeting at an end is not crossing");
assert.ok(!crosses([[0, 0], [10, 0]], [[0, 5], [10, 5]]));

// ---- shading ----
assert.deepEqual(shadedStates([{ state: "Assam" }, { state: "Assam" }, { state: "Goa" }], [{ id: "as", name: "Assam" }, { id: "br", name: "Bihar" }, { id: "ga", name: "Goa" }]), ["as", "ga"]);
assert.deepEqual(shadedStates(undefined, [{ id: "as", name: "Assam" }]), []);

// ---- every dot map and the states map on India's outline ----
const read = (f: string) => JSON.parse(readFileSync(new URL(f, import.meta.url), "utf8"));
const india = read("./data/india.json") as { w: number; h: number; states: { id: string; name: string; d: string }[] };
const maps: MapLayer[] = readdirSync(new URL("./data/maps/", import.meta.url)).filter((f) => f.endsWith(".json") && f !== "index.json").map((f) => read(`./data/maps/${f}`));
const frame: Frame = { w: india.w, top: LAYOUT.top, bottom: india.h - 20, pad: LAYOUT.pad, maxChars: LAYOUT.maxChars, lineH: LAYOUT.lineH, gap: LAYOUT.gap };
const base = { project, shapes: india.states };

let checked = 0;
for (const map of maps.filter((m) => labellable(m))) {
  const pts = printPoints(map, base);
  const { labelled } = chooseLabelled(pts, map.kind);
  const spots: Spot[] = labelled.map((p) => ({ id: String(p.n), name: p.name, x: p.x, y: p.y }));
  const out = layoutCallouts(spots, frame);
  assert.equal(out.length, spots.length, `${map.id}: every named place gets a name`);

  for (const side of ["left", "right"] as const) {
    const col = out.filter((o) => o.side === side).sort((a, b) => a.ly - b.ly);
    for (let i = 0; i < col.length; i++) {
      assert.ok(col[i].ly - col[i].h / 2 >= frame.top - 0.01 && col[i].ly + col[i].h / 2 <= frame.bottom + 0.01, `${map.id}: a ${side} name is inside the frame`);
      if (i) assert.ok(col[i].ly - col[i - 1].ly >= (col[i].h + col[i - 1].h) / 2 + frame.gap - 0.01, `${map.id}: ${side} names do not touch`);
    }
    // The names run in the order of their dots from top to bottom, which is what keeps the lines apart.
    const byDot = [...col].sort((a, b) => a.spot.y - b.spot.y || a.spot.x - b.spot.x);
    assert.deepEqual(byDot.map((o) => o.spot.id), col.map((o) => o.spot.id), `${map.id}: ${side} order follows the dots`);
  }
  for (const o of out) {
    assert.deepEqual(o.path[0], [o.spot.x, o.spot.y], `${map.id}: a line starts on its dot`);
    assert.ok(o.lines.length <= 2 && o.lines.every((l) => l.length <= frame.maxChars));
  }
  // No two lines cross.
  const segs = out.flatMap((o) => [[o.path[0], o.path[1]], [o.path[1], o.path[2]]] as [[number, number], [number, number]][]);
  for (let i = 0; i < segs.length; i++) for (let j = i + 1; j < segs.length; j++) assert.ok(!crosses(segs[i], segs[j]), `${map.id}: two lines cross`);
  checked++;
}
assert.ok(checked >= 20, `${checked} maps checked`);

console.log("map-labels: ok");
