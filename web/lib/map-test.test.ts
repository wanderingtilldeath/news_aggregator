// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { DEFAULT_TOLERANCE_KM, LINE_TOLERANCE, distKm, distToPath, judgePlace, parseScores, pickQuestions, questionsFor, recordScore, summarise, type Question } from "./map-test.ts";
import type { MapLayer } from "./maps.ts";

const states = [{ id: "kerala", name: "Kerala" }, { id: "goa", name: "Goa" }, { id: "assam", name: "Assam" }];
const base = { category: "wildlife", blurb: "", base: "india" as const, source: "s", licence: "l", asOf: "2026-11" };

// What each kind of map asks.
const points: MapLayer = { ...base, id: "p", title: "P", kind: "point", items: [{ id: "a", name: "Kaziranga", lon: 93.4, lat: 26.6, state: "Assam" }, { id: "b", name: "Periyar", lon: 77.2, lat: 9.5, state: "Kerala" }] };
assert.deepEqual(questionsFor(points, states).map((q) => [q.kind, q.ask, q.hint]), [["place", "Find Kaziranga", "In Assam"], ["place", "Find Periyar", "In Kerala"]]);
// A map may word its own question when the name would give the answer away; the label is the name shown on the answer.
const caps: MapLayer = { ...base, id: "c", title: "C", kind: "point", base: "world", items: [{ id: "paris", name: "Paris", lon: 2.35, lat: 48.86, state: "", ask: "Find the capital of France" }] };
assert.deepEqual(questionsFor(caps, states).map((q) => [q.ask, (q as { label: string }).label, q.hint]), [["Find the capital of France", "Paris", ""]]);
const soils: MapLayer = { ...base, id: "soils", title: "Soils", kind: "region", types: [], regions: { kerala: "laterite", goa: "laterite" } };
assert.deepEqual(questionsFor(soils, states).map((q) => [q.kind, q.id, (q as { answer: string }).answer]), [["type", "kerala", "laterite"], ["type", "goa", "laterite"]], "a state with no type is not asked");
assert.equal(questionsFor({ ...base, id: "s", title: "S", kind: "states" }, states).length, 3);
assert.deepEqual(questionsFor({ ...base, id: "l", title: "L", kind: "line", lines: [] }, states), [], "a map nothing can be asked about gives no questions");
const rivers: MapLayer = { ...base, id: "r", title: "R", kind: "line", lines: [{ id: "ganga", name: "Ganga", d: "M0 0L100 0", type: "himalayan" }] };
assert.deepEqual(questionsFor(rivers, states).map((q) => [q.kind, q.ask, q.hint]), [["line", "Find the Ganga", "A himalayan river"]]);

// Taps near a line: the distance to the nearest point of the path (not just a vertex), over several parts of a path too.
assert.equal(distToPath("M0 0L100 0", 50, 0), 0);
assert.equal(distToPath("M0 0L100 0", 50, 7), 7, "distance to the middle of a segment, not to a corner");
assert.equal(distToPath("M0 0L100 0", -30, 40), 50, "beyond the end it is the distance to the end");
assert.equal(distToPath("M0 0L10 0M50 50L60 50", 55, 52), 2, "the nearest of several parts");
assert.equal(distToPath("M5 5", 8, 9), 5, "a path of one point");
assert.ok(distToPath("M0 0L100 0", 50, LINE_TOLERANCE) <= LINE_TOLERANCE && distToPath("M0 0L100 0", 50, LINE_TOLERANCE + 1) > LINE_TOLERANCE);

// Picking: n without repeats, all of them when there are fewer, the same answer for the same random numbers.
const all = Array.from({ length: 30 }, (_, i) => i);
let seed = 7;
const rand = () => ((seed = (seed * 16807) % 2147483647) / 2147483647);
const picked = pickQuestions(all, 10, rand);
assert.equal(picked.length, 10);
assert.equal(new Set(picked).size, 10);
assert.equal(pickQuestions([1, 2, 3], 10, rand).length, 3);
seed = 7;
assert.deepEqual(pickQuestions(all, 10, rand), picked);
assert.deepEqual(all, Array.from({ length: 30 }, (_, i) => i), "the list given is not shuffled in place");

// Distance: known pairs, symmetric, and judged against the map's tolerance (100 km when none).
assert.equal(distKm({ lon: 77.2, lat: 28.6 }, { lon: 77.2, lat: 28.6 }), 0);
const delhiMumbai = distKm({ lon: 77.2, lat: 28.6 }, { lon: 72.88, lat: 19.07 });
assert.ok(delhiMumbai > 1100 && delhiMumbai < 1200, String(delhiMumbai));
assert.equal(distKm({ lon: 1, lat: 2 }, { lon: 5, lat: 6 }), distKm({ lon: 5, lat: 6 }, { lon: 1, lat: 2 }));
const q = questionsFor(points, states)[0] as Extract<Question, { kind: "place" }>;
assert.deepEqual(judgePlace(q, { lon: 93.4, lat: 26.6 }), { km: 0, correct: true });
assert.equal(judgePlace(q, { lon: 93.4, lat: 26.6 + 1.0 }).correct, false, "111 km is outside 100");
assert.equal(judgePlace(q, { lon: 93.4, lat: 26.6 + 1.0 }, 150).correct, true, "but inside 150");
assert.equal(DEFAULT_TOLERANCE_KM, 100);

// The summary lists what was missed.
const sum = summarise([{ id: "a", ask: "A", correct: true }, { id: "b", ask: "B", correct: false, km: 400 }, { id: "c", ask: "C", correct: false }]);
assert.deepEqual([sum.score, sum.total, sum.misses.map((m) => m.id)], [1, 3, ["b", "c"]]);

// Scores: bad storage reads as empty; the best result of a map is kept and plays are counted.
assert.deepEqual(parseScores(""), {});
assert.deepEqual(parseScores("not json"), {});
assert.deepEqual(parseScores("[1]"), {});
assert.deepEqual(parseScores(JSON.stringify({ ok: { best: 7, total: 10, plays: 2, at: 1 }, bad: { best: "x" }, nope: null })), { ok: { best: 7, total: 10, plays: 2, at: 1 } });
let s = recordScore({}, "tiger", 6, 10, 100);
assert.deepEqual(s.tiger, { best: 6, total: 10, plays: 1, at: 100 });
s = recordScore(s, "tiger", 4, 10, 200);
assert.deepEqual(s.tiger, { best: 6, total: 10, plays: 2, at: 200 }, "a worse test keeps the best");
s = recordScore(s, "tiger", 9, 10, 300);
assert.equal(s.tiger.best, 9);
s = recordScore(s, "tiger", 3, 3, 400);
assert.deepEqual([s.tiger.best, s.tiger.total], [3, 3], "3 of 3 is a better share than 9 of 10");
s = recordScore(s, "tiger", 9, 10, 500);
assert.deepEqual([s.tiger.best, s.tiger.total], [3, 3], "9 of 10 is not a better share than 3 of 3");

// A history map shows what happened at a place after the answer; other maps carry no note.
const hist: MapLayer = { ...points, category: "history", items: [{ id: "p", name: "Plassey", lon: 88.25, lat: 23.8, state: "West Bengal", detail: "1757: Clive defeats Siraj ud-Daulah.", era: "1750–1947" }] };
const hq = questionsFor(hist, [])[0];
assert.equal(hq.kind === "place" && hq.note, "1757: Clive defeats Siraj ud-Daulah.");
const nq = questionsFor({ ...hist, category: "wildlife" }, [])[0];
assert.equal(nq.kind === "place" && nq.note, undefined);

console.log("map-test: ok");
