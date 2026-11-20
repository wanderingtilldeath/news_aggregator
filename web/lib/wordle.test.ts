// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { ANSWERS, WORD_LENGTH, dayKey, dayNumber, nextStats, readStats, score, shareText, wordOfDay } from "./wordle.ts";

// The answer list: five lowercase letters each, no repeats.
assert.ok(ANSWERS.length > 150);
assert.ok(ANSWERS.every((w) => /^[a-z]+$/.test(w) && w.length === WORD_LENGTH));
assert.equal(new Set(ANSWERS).size, ANSWERS.length);

// Scoring, including repeated letters.
assert.deepEqual(score("crane", "crane"), ["correct", "correct", "correct", "correct", "correct"]);
assert.deepEqual(score("audit", "voter"), ["absent", "absent", "absent", "absent", "present"]);
assert.deepEqual(score("speed", "abide"), ["absent", "absent", "present", "absent", "present"]); // one e in the answer's reach: only the first e is marked
assert.deepEqual(score("eerie", "there"), ["present", "absent", "present", "absent", "correct"]);

// The day.
assert.equal(dayKey(new Date(Date.UTC(2026, 10, 20, 23, 59))), "2026-11-20");
assert.equal(dayNumber("2026-01-01"), 0);
assert.equal(dayNumber("2026-01-02"), 1);
assert.equal(wordOfDay("2026-01-01"), ANSWERS[0]);
assert.equal(wordOfDay("2026-11-20"), ANSWERS[323 % ANSWERS.length]); // 2026-11-20 is day 323

// Share text hides the word.
const shared = shareText(["audit", "voter"], "voter", "2026-01-03");
assert.equal(shared, "Postpaper Wordle 2 2/6\n⬛⬛⬛⬛🟨\n🟩🟩🟩🟩🟩");
assert.ok(!shared.includes("voter"));
assert.ok(shareText(Array(6).fill("audit"), "voter", "2026-01-03").startsWith("Postpaper Wordle 2 X/6"));

// Stats: a win the next day extends the streak, a gap restarts it, a loss resets it.
let st = nextStats("", true, "2026-03-01");
assert.deepEqual(readStats(st), { played: 1, won: 1, streak: 1 });
st = nextStats(st, true, "2026-03-02");
assert.equal(readStats(st).streak, 2);
st = nextStats(st, true, "2026-03-05");
assert.equal(readStats(st).streak, 1);
st = nextStats(st, false, "2026-03-06");
assert.deepEqual(readStats(st), { played: 4, won: 3, streak: 0 });

console.log("wordle: all checks passed");
