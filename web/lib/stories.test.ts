// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { oneEach, sameStory } from "./stories.ts";

const it = (id: string, story: string | undefined, importance: number | undefined, hour: number) => ({ id, story, importance, time: `2026-11-22T${String(hour).padStart(2, "0")}:00:00Z` });
const ids = (xs: { id: string }[]) => xs.map((x) => x.id).join(",");

// The Guardian and Al Jazeera on one event, plus a story on its own: one card for the event.
assert.equal(ids(oneEach([it("guardian", "g1", 5, 9), it("meta", undefined, 5, 8), it("aljazeera", "g1", 5, 7)])), "guardian,meta");

// The better-ranked one stands for the group, wherever it sits; the rest keep the input order.
assert.equal(ids(oneEach([it("a", "g1", 3, 9), it("b", "x", 4, 8), it("c", "g1", 5, 7)])), "b,c");
// Equal importance: the newest.
assert.equal(ids(oneEach([it("old", "g1", 4, 6), it("new", "g1", 4, 9)])), "new");
// An unscored story counts as 2.
assert.equal(ids(oneEach([it("unscored", "g1", undefined, 9), it("scored", "g1", 3, 5)])), "scored");
assert.equal(ids(oneEach([it("unscored", "g1", undefined, 9), it("two", "g1", 2, 5)])), "unscored");

// Stories with no group are never merged, even with the same importance and time.
assert.equal(ids(oneEach([it("a", undefined, 3, 9), it("b", undefined, 3, 9), it("c", "", 3, 9)])), "a,b,c");
// Several groups at once, and an empty list.
assert.equal(ids(oneEach([it("a1", "A", 3, 9), it("b1", "B", 3, 9), it("a2", "A", 4, 9), it("b2", "B", 2, 9)])), "b1,a2");
assert.deepEqual(oneEach([]), []);

assert.equal(sameStory(it("a", "g1", 1, 1), it("b", "g1", 1, 1)), true);
assert.equal(sameStory(it("a", undefined, 1, 1), it("b", undefined, 1, 1)), false);
assert.equal(sameStory(it("a", "g1", 1, 1), it("b", "g2", 1, 1)), false);

console.log("stories: ok");
