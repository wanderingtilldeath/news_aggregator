// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { MAX_NOTES, MAX_TEXT, OWN_STORY, PRO_MAX_NOTES, allTags, cleanTag, edited, filterNotes, noteId, ownNote, parseNotes, parseTags, toMarkdown, toggled, tokenFromInput, type Note } from "./notebook.ts";

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


// A note the reader types: trimmed, kept as typed (line breaks too), cut to the limit, never empty, never two with one id.
const mine = ownNote("  Revise the repo rate\nand inflation  ", 1_700_000_000_000, "ab12");
assert.equal(mine?.text, "Revise the repo rate\nand inflation");
assert.equal(mine?.story, OWN_STORY);
assert.match(mine!.id, /^own:[a-z0-9]+-ab12$/);
assert.equal(ownNote("   \n ", 1, "x"), null);
assert.equal(ownNote("x".repeat(MAX_TEXT + 500), 1, "x")?.text.length, MAX_TEXT);
assert.notEqual(ownNote("a", 1, "aaaa")!.id, ownNote("a", 2, "aaaa")!.id);
assert.notEqual(ownNote("a", 1, "aaaa")!.id, ownNote("a", 1, "bbbb")!.id);
assert.equal(ownNote("a", 1, "a b!c/d")!.id.endsWith("-abcd"), true, "odd characters in the random part are dropped");
// An own note is in the list like any other and can be removed with the same call.
assert.deepEqual(toggled([note("a")], mine!).map((n) => n.id), [mine!.id, "a"]);

// Editing a saved sentence keeps its id, story and title, and changes only the text.
const base = [note("a"), note("b", { note: "my comment", tags: ["x"] })];
const fixed = edited(base, "b", "  A reworded sentence  ");
assert.equal(fixed?.[1].text, "A reworded sentence");
assert.deepEqual([fixed?.[1].id, fixed?.[1].story, fixed?.[1].title, fixed?.[1].note, fixed?.[1].tags], ["b", "s1", "Budget story", "my comment", ["x"]]);
assert.equal(fixed?.[0], base[0], "other notes are untouched");
assert.equal(base[1].text, "Sentence b", "the input is not changed");
assert.equal(edited(base, "b", "   "), null, "an empty text is refused");
assert.equal(edited(base, "b", "Sentence b"), null, "no change, nothing to save");
assert.equal(edited(base, "missing", "text"), null);
assert.equal(edited(base, "a", "y".repeat(MAX_TEXT + 10))?.[0].text.length, MAX_TEXT);

// The export links a story's heading to its page, but an own-notes heading is plain text.
const mdOwn = toMarkdown([note("a"), mine!], "https://site.test");
assert.match(mdOwn, /## \[Budget story\]\(https:\/\/site\.test\/story\/s1\)/);
assert.match(mdOwn, /## My own notes\n\n- Revise the repo rate and inflation/);
assert.ok(!mdOwn.includes("/story/own"), "no link for the own-notes heading");

console.log("notebook: all checks passed");
