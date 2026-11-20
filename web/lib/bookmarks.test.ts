// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_BOOKMARKS, parseBookmarks, toggled, type Bookmark } from "./bookmarks.ts";

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

console.log("bookmarks: all checks passed");
