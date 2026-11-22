// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { KINDS, changed, hash, plan, toStored, toWire, type Base, type Kind } from "./account-sync-core.ts";

const empty = (): Record<Kind, string> => ({ bookmarks: "", cards: "", streak: "", missed: "" });

// A device that has not met this account merges what it holds, leaving out empty kinds.
assert.deepEqual(plan({ ...empty(), streak: "2026-11-22|2" }, {}, "alice", ""), { mode: "merge", kinds: ["streak"] });
assert.equal(plan(empty(), {}, "alice", "bob").mode, "merge", "another user's leftovers are merged, not trusted");

// After that: nothing changed -> read; something changed -> replace just that kind.
const locals = { ...empty(), bookmarks: '[{"id":"a"}]' };
const base: Base = { bookmarks: hash(locals.bookmarks), cards: hash(""), streak: hash(""), missed: hash("") };
assert.deepEqual(plan(locals, base, "alice", "alice"), { mode: "get", kinds: [] });
assert.deepEqual(plan({ ...locals, cards: '{"c":{"box":1,"due":2}}' }, base, "alice", "alice"), { mode: "replace", kinds: ["cards"] });
assert.deepEqual(changed({ ...locals, bookmarks: "" }, base), ["bookmarks"], "clearing a kind is a change too");
assert.deepEqual(changed(empty(), {}), [], "a clean device with no baseline has nothing to push");

// The fingerprint tells similar strings apart.
assert.notEqual(hash("[]"), hash("{}"));
assert.equal(hash("same"), hash("same"));

// Wire and stored forms: bad storage reads as empty, empty server values store as "".
assert.deepEqual(toWire("bookmarks", "not json"), []);
assert.deepEqual(toWire("cards", "[1]"), {});
assert.equal(toWire("streak", "2026-11-22|3"), "2026-11-22|3");
assert.equal(toStored("bookmarks", []), "");
assert.equal(toStored("cards", {}), "");
assert.equal(toStored("cards", { a: { box: 1, due: 2 } }), '{"a":{"box":1,"due":2}}');
assert.equal(toStored("streak", null), "");
assert.equal(KINDS.length, 4);
console.log("account-sync-core: ok");
