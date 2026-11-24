// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_MARKS, MAX_NAME, addNote, clearMap, exportText, isStarred, marksFor, onlyStarred, parseMarks, removeMark, restoreMark, starMissed, starredItems, toggleStar, updateMark } from "./map-marks.ts";

const spot = (id: string, name = id) => ({ id, name, lon: 80, lat: 20 });

// Starring: on, off (the star and its note go), once only, and only while there is room.
let m = toggleStar([], "tiger", "india", spot("kanha", "Kanha Tiger Reserve"), 100);
assert.equal(m.length, 1);
assert.deepEqual([m[0].id, m[0].item, m[0].from, m[0].map], ["tiger:kanha", "kanha", "star", "tiger"]);
assert.ok(isStarred(m, "tiger", "kanha") && !isStarred(m, "dams", "kanha"), "a star belongs to one map");
m = updateMark(m, "tiger:kanha", { note: "Tigers, Madhya Pradesh" });
assert.equal(m[0].note, "Tigers, Madhya Pradesh");
assert.deepEqual(toggleStar(m, "tiger", "india", spot("kanha"), 200), [], "a second press takes the star off");
const full = Array.from({ length: MAX_MARKS }, (_, i) => toggleStar([], "x", "india", spot(`p${i}`), i)[0]);
assert.equal(toggleStar(full, "x", "india", spot("extra"), 1).length, MAX_MARKS);

// A missed test question stars the place, once, tagged as from a test.
let t = starMissed([], "tiger", "india", [spot("a"), spot("b")], 5);
t = starMissed(t, "tiger", "india", [spot("b"), spot("c")], 6);
assert.deepEqual(t.map((x) => x.item).sort(), ["a", "b", "c"]);
assert.ok(t.every((x) => x.from === "test"));

// Free notes: inside the outline of their map only, one id once, name and note clipped.
const note = { id: "n-1", map: "tiger", base: "india" as const, lon: 80, lat: 20, name: "x".repeat(200), note: "hook" };
const n = addNote([], note, 1);
assert.equal(n[0].name.length, MAX_NAME);
assert.equal(addNote(n, note, 2).length, 1);
assert.equal(addNote([], { ...note, id: "n-2", lon: 0, lat: 0 }, 1).length, 0, "off the India outline");
assert.equal(addNote([], { ...note, id: "n-3", base: "world", lon: 10, lat: 50 }, 1).length, 1, "on the world map");
assert.equal(addNote([], { ...note, id: "n-4", base: "world", lon: 10, lat: 89 }, 1).length, 0, "off the world picture");

// Each map shows its own marks; "all maps" shows the ones on the same outline; old unscoped notes show on every India map.
const mix = [...t, ...addNote([], { ...note, map: "dams" }, 9), ...addNote([], { ...note, id: "w", map: "world-seas", base: "world" as const, lon: 10, lat: 50 }, 9), { ...n[0], id: "old", map: "" }];
assert.deepEqual(marksFor(mix, "tiger", "india").map((x) => x.id).sort(), ["a".replace("a", "tiger:a"), "tiger:b", "tiger:c", "old"].sort());
assert.equal(marksFor(mix, "tiger", "india", true).length, 5, "all India marks");
assert.equal(marksFor(mix, "world-seas", "world").length, 1);
assert.equal(clearMap(mix, "tiger").length, 3);

// Remove and undo put it back in place by time.
const gone = removeMark(mix, "tiger:b");
assert.equal(gone.length, mix.length - 1);
assert.deepEqual(restoreMark(gone, mix.find((x) => x.id === "tiger:b")!).map((x) => x.id).sort(), mix.map((x) => x.id).sort());
assert.equal(restoreMark(mix, mix[0]).length, mix.length, "restoring one that is there changes nothing");

// Starred ids and the map cut down to them.
assert.deepEqual([...starredItems(t, "tiger")].sort(), ["a", "b", "c"]);
assert.deepEqual(onlyStarred({ items: [{ id: "a" }, { id: "z" }], lines: [{ id: "r" }] }, new Set(["a", "r"])), { items: [{ id: "a" }], lines: [{ id: "r" }] });
assert.deepEqual(onlyStarred({ items: [{ id: "a" }] }, new Set<string>()), { items: [] });

// Reading storage: junk is dropped, newest first, and the earlier single list is carried over once.
assert.deepEqual(parseMarks("not json"), []);
assert.deepEqual(parseMarks('[{"id":"x"}, 5, null]'), []);
assert.deepEqual(parseMarks(JSON.stringify(mix)).map((x) => x.at), [...mix.map((x) => x.at)].sort((a, b) => b - a));
const legacy = JSON.stringify([{ id: "tiger:kanha", lon: 80.5, lat: 22, label: "Kanha", note: "hook" }, { id: "m-abc", lon: 77, lat: 28, label: "Delhi", note: "" }]);
const old = parseMarks("", legacy);
assert.deepEqual(old.map((x) => [x.id, x.map, x.item, x.from]), [["tiger:kanha", "tiger", "kanha", "star"], ["m-abc", "", undefined, "note"]]);
assert.deepEqual(parseMarks("[]", legacy), [], "once something is stored the old list is not read again");

// The text to save.
assert.match(exportText([{ ...t[0], note: "mnemonic" }], () => "Tiger reserves"), /Tiger reserves[\s\S]*mnemonic/);
assert.equal(exportText([], () => ""), "No notes yet.\n");

console.log("map-marks: ok");
