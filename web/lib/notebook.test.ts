// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_NOTES, PRO_MAX_NOTES, allTags, cleanTag, filterNotes, noteId, parseNotes, parseTags, toMarkdown, toggled, tokenFromInput, type Note } from "./notebook.ts";

const note = (id: string, extra: Partial<Note> = {}): Note => ({ id, text: `Sentence ${id}`, story: "s1", title: "Budget story", at: 1, ...extra });

// Pressing the star adds a note at the front; pressing again removes it.
let list = toggled([], note("a"));
list = toggled(list, note("b"));
assert.deepEqual(list.map((n) => n.id), ["b", "a"]);
assert.deepEqual(toggled(list, note("a")).map((n) => n.id), ["b"]);

// A free notebook stops at 300; a Pro one at 10,000.
const full = Array.from({ length: MAX_NOTES }, (_, i) => note(`n${i}`));
assert.equal(toggled(full, note("new")).length, MAX_NOTES);
assert.equal(toggled(full, note("new"))[0].id, "new");
assert.equal(toggled(full, note("new"), PRO_MAX_NOTES).length, MAX_NOTES + 1);

// The same sentence of the same story is the same note.
assert.equal(noteId("s1", "Hello"), noteId("s1", "Hello"));
assert.notEqual(noteId("s1", "Hello"), noteId("s2", "Hello"));

// Older notes, without the Pro fields, still read.
assert.deepEqual(parseNotes(JSON.stringify([{ id: "x", text: "t", story: "s", title: "T", at: 1 }])).map((n) => n.note), [undefined]);
assert.deepEqual(parseNotes("not json"), []);

// Tags are short lower-case words, as the server cleans them.
assert.equal(cleanTag(" UPSC prep "), "upsc-prep");
assert.equal(cleanTag("a__b"), "a-b");
assert.equal(cleanTag("x!y"), "xy");
assert.equal(cleanTag("!!!"), "");
assert.equal(cleanTag("a".repeat(40)).length, 24);
assert.deepEqual(parseTags("Economy, upsc prep, economy, , x!y"), ["economy", "upsc-prep", "xy"]);
assert.equal(parseTags(Array.from({ length: 15 }, (_, i) => `t${i}`).join(",")).length, 10);

// Tags in use, the most used first.
const tagged = [note("a", { tags: ["economy", "exam"] }), note("b", { tags: ["economy"] }), note("c")];
assert.deepEqual(allTags(tagged), ["economy", "exam"]);

// Search finds every word in the sentence, the member's note, the title or a tag; a tag narrows it.
const corpus = [
  note("a", { text: "The repo rate stays at 6.5%", note: "ask about inflation", tags: ["economy"] }),
  note("b", { text: "Monsoon sets in early", title: "Weather desk", tags: ["exam"] }),
];
assert.deepEqual(filterNotes(corpus, "repo rate", "").map((n) => n.id), ["a"]);
assert.deepEqual(filterNotes(corpus, "INFLATION", "").map((n) => n.id), ["a"]);
assert.deepEqual(filterNotes(corpus, "weather", "").map((n) => n.id), ["b"]);
assert.deepEqual(filterNotes(corpus, "", "exam").map((n) => n.id), ["b"]);
assert.deepEqual(filterNotes(corpus, "repo", "exam").map((n) => n.id), []);
assert.equal(filterNotes(corpus, "  ", "").length, 2);

// A member pastes their Pro link or the bare token.
const token = "abcDEF123456_-xyzABC123456";
assert.equal(tokenFromInput(`https://www.postpaper.in/pro/edition?token=${token}&day=2026-11-21`), token);
assert.equal(tokenFromInput(`  ${token}  `), token);
assert.equal(tokenFromInput("short"), "");
assert.equal(tokenFromInput("https://x.test/pro?nothing=1"), "");
assert.equal(tokenFromInput("a b c d e f g h i j k l m n o p"), "");

// The Markdown export carries the member's note and tags.
const md = toMarkdown([note("a", { note: "my thought", tags: ["economy"] }), note("b")], "https://site.test");
assert.match(md, /- Sentence a #economy\n {2}- My note: my thought/);
assert.match(md, /- Sentence b\n/);
assert.match(md, /## \[Budget story\]\(https:\/\/site\.test\/story\/s1\)/);

console.log("notebook: all checks passed");
