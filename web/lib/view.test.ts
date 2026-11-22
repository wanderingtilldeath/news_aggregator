// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_PAGE, MAX_QUERY, parseView } from "./view.ts";

// A normal link is read as before.
assert.deepEqual(parseView({ days: "7", category: "India", source: "www.thehindu.com", q: " rbi ", page: "3", track: "upsc", mode: "1" }), {
  days: "7", category: "india", source: "www.thehindu.com", q: "rbi", page: 3, track: "upsc", mode: "1",
});

// Unknown or odd values fall back to the defaults.
assert.deepEqual(parseView({}), { days: "2", category: "", source: "", q: "", page: 1, track: "", mode: "" });
assert.equal(parseView({ days: "999" }).days, "2");
assert.equal(parseView({ page: "-4" }).page, 1);
assert.equal(parseView({ page: "abc" }).page, 1);

// The values that make a cache key are bounded.
assert.equal(parseView({ q: "x".repeat(500) }).q.length, MAX_QUERY);
assert.equal(parseView({ q: "a".repeat(79) + " " + "b".repeat(50) }).q, "a".repeat(79)); // no trailing space after the cut
assert.equal(parseView({ page: "100000" }).page, MAX_PAGE);
assert.equal(parseView({ source: "a".repeat(61) }).source, "");
assert.equal(parseView({ source: "evil host/<script>" }).source, "");
assert.equal(parseView({ source: "indianexpress.com" }).source, "indianexpress.com");
assert.equal(parseView({ category: "no spaces allowed" }).category, "");
assert.equal(parseView({ category: "a".repeat(25) }).category, "");
assert.equal(parseView({ category: "gs3-economy" }).category, "gs3-economy");

// A repeated parameter uses the first value.
assert.equal(parseView({ page: ["2", "9"] }).page, 2);

console.log("view: ok");
