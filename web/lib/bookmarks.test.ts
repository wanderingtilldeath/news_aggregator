// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_BOOKMARKS, filterBookmarks, groupBySaved, parseBookmarks, toggled, type Bookmark } from "./bookmarks.ts";

const mark = (id: string): Bookmark => ({ id, title: `Story ${id}`, source: "x.test", time: "2026-11-20T08:00:00Z", at: 1 });

// Adding puts the story first; pressing again takes it out.
let list = toggled([], mark("a"));
list = toggled(list, mark("b"));
assert.deepEqual(list.map((b) => b.id), ["b", "a"]);
assert.deepEqual(toggled(list, mark("a")).map((b) => b.id), ["b"]);

// The list is capped, dropping the oldest.
let many: Bookmark[] = [];
for (let i = 0; i < MAX_BOOKMARKS + 5; i++) many = toggled(many, mark(String(i)));
assert.equal(many.length, MAX_BOOKMARKS);
assert.equal(many[0].id, String(MAX_BOOKMARKS + 4));

// Bad or foreign storage never throws: it reads as an empty list, and a broken entry is dropped.
assert.deepEqual(parseBookmarks(""), []);
assert.deepEqual(parseBookmarks("not json"), []);
assert.deepEqual(parseBookmarks('{"id":"a"}'), []);
assert.deepEqual(parseBookmarks(JSON.stringify([mark("a"), { id: 5 }, null])).map((b) => b.id), ["a"]);

// Search: every word, in the title or the publisher, any case; an empty query keeps everything.
const found = [
  { ...mark("a"), title: "RBI holds the repo rate", source: "www.thehindu.com" },
  { ...mark("b"), title: "Monsoon sets in early", source: "indianexpress.com" },
];
assert.deepEqual(filterBookmarks(found, "repo RATE").map((b) => b.id), ["a"]);
assert.deepEqual(filterBookmarks(found, "indianexpress").map((b) => b.id), ["b"]);
assert.deepEqual(filterBookmarks(found, "repo monsoon"), []);
assert.equal(filterBookmarks(found, "   ").length, 2);

// Grouping by the day saved, on the reader's clock: today, yesterday, the days before in the week, earlier.
const noon = (daysAgo: number) => new Date(2026, 10, 23 - daysAgo, 12, 0, 0).getTime();
const now = new Date(2026, 10, 23, 18, 30, 0).getTime();
const saved = [
  { ...mark("today"), at: noon(0) },
  { ...mark("late-last-night"), at: new Date(2026, 10, 22, 23, 59, 0).getTime() },
  { ...mark("three"), at: noon(3) },
  { ...mark("six"), at: noon(6) },
  { ...mark("seven"), at: noon(7) },
  { ...mark("old"), at: noon(40) },
];
const groups = groupBySaved(saved, now);
assert.deepEqual(groups.map((g) => g.label), ["Today", "Yesterday", "Earlier this week", "Earlier"]);
assert.deepEqual(groups.map((g) => g.items.map((b) => b.id).join(",")), ["today", "late-last-night", "three,six", "seven,old"]);
assert.deepEqual(groupBySaved([{ ...mark("only"), at: noon(40) }], now).map((g) => g.label), ["Earlier"], "an empty group is left out");
assert.deepEqual(groupBySaved([], now), []);
assert.equal(groupBySaved([{ ...mark("future"), at: now + 86400000 * 3 }], now)[0].label, "Today", "a clock set backwards counts as today");

console.log("bookmarks: all checks passed");
