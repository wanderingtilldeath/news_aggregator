// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_MARKS, MAX_NAME, clearMap, isStarred, marksFor, onlyStarred, parseMarks, removeMark, restoreMark, starMissed, starredItems, toggleStar } from "./map-marks.ts";

const spot = (id: string, name = id) => ({ id, name, lon: 80, lat: 20 });

// Starring: on, off, once only, and only while there is room.
const m = toggleStar([], "tiger", "india", spot("kanha", "Kanha Tiger Reserve"), 100);
assert.equal(m.length, 1);
assert.deepEqual([m[0].id, m[0].item, m[0].from, m[0].map], ["tiger:kanha", "kanha", "star", "tiger"]);
assert.ok(isStarred(m, "tiger", "kanha") && !isStarred(m, "dams", "kanha"), "a star belongs to one map");
assert.equal(toggleStar([], "x", "india", spot("p", "x".repeat(200)), 1)[0].name.length, MAX_NAME, "the name is clipped");
assert.deepEqual(toggleStar(m, "tiger", "india", spot("kanha"), 200), [], "a second press takes the star off");
const full = Array.from({ length: MAX_MARKS }, (_, i) => toggleStar([], "x", "india", spot(`p${i}`), i)[0]);
assert.equal(toggleStar(full, "x", "india", spot("extra"), 1).length, MAX_MARKS);

// A missed test question stars the place, once, tagged as from a test.
let t = starMissed([], "tiger", "india", [spot("a"), spot("b")], 5);
t = starMissed(t, "tiger", "india", [spot("b"), spot("c")], 6);
assert.deepEqual(t.map((x) => x.item).sort(), ["a", "b", "c"]);
assert.ok(t.every((x) => x.from === "test"));

// Each map shows its own stars; "all maps" shows the ones on the same outline.
const world = toggleStar([], "world-seas", "world", { id: "s", name: "Sea", lon: 10, lat: 50 }, 9);
const mix = [...t, ...toggleStar([], "dams", "india", spot("d"), 9), ...world];
assert.deepEqual(marksFor(mix, "tiger", "india").map((x) => x.id).sort(), ["tiger:a", "tiger:b", "tiger:c"]);
assert.equal(marksFor(mix, "tiger", "india", true).length, 4, "all India stars");
assert.equal(marksFor(mix, "world-seas", "world").length, 1);
assert.equal(clearMap(mix, "tiger").length, 2);

// Remove and undo put it back in place by time.
const gone = removeMark(mix, "tiger:b");
assert.equal(gone.length, mix.length - 1);
assert.deepEqual(restoreMark(gone, mix.find((x) => x.id === "tiger:b")!).map((x) => x.id).sort(), mix.map((x) => x.id).sort());
assert.equal(restoreMark(mix, mix[0]).length, mix.length, "restoring one that is there changes nothing");

// Starred ids and the map cut down to them.
assert.deepEqual([...starredItems(t, "tiger")].sort(), ["a", "b", "c"]);
assert.deepEqual(onlyStarred({ items: [{ id: "a" }, { id: "z" }], lines: [{ id: "r" }] }, new Set(["a", "r"])), { items: [{ id: "a" }], lines: [{ id: "r" }] });
assert.deepEqual(onlyStarred({ items: [{ id: "a" }] }, new Set<string>()), { items: [] });

// Reading storage: junk is dropped, newest first, and free notes of an earlier version are gone.
assert.deepEqual(parseMarks("not json"), []);
assert.deepEqual(parseMarks('[{"id":"x"}, 5, null]'), []);
const oldNote = { id: "n-1", map: "tiger", base: "india", lon: 80, lat: 20, name: "free note", note: "hook", from: "note", at: 1 };
assert.deepEqual(parseMarks(JSON.stringify([oldNote])), [], "a free note has no place of a map");
const keptStar = { ...oldNote, id: "tiger:kanha", item: "kanha", from: "star", note: "an old hook" };
const read = parseMarks(JSON.stringify([oldNote, keptStar]));
assert.equal(read.length, 1);
assert.equal("note" in read[0], false, "an old hook is not kept");
assert.deepEqual(parseMarks(JSON.stringify(mix)).map((x) => x.at), [...mix.map((x) => x.at)].sort((a, b) => b - a));
const legacy = JSON.stringify([{ id: "tiger:kanha", lon: 80.5, lat: 22, label: "Kanha", note: "hook" }, { id: "m-abc", lon: 77, lat: 28, label: "Delhi", note: "" }]);
assert.deepEqual(parseMarks("", legacy).map((x) => [x.id, x.map, x.item, x.from]), [["tiger:kanha", "tiger", "kanha", "star"]], "only what was made from a map's list is carried over");
assert.deepEqual(parseMarks("[]", legacy), [], "once something is stored the old list is not read again");

console.log("map-marks: ok");
