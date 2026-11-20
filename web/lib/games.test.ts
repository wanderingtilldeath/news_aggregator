// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { arrow, makeBee, makeLadder, beePoints, beeRank, beeWords, blankOut, closeness, countryOfDay, distanceKm, headlineOfDay, isPangram, isStep, maskName, whoOfDay, type BeePuzzle, type Country, type Ladder } from "./games.ts";

const data = (f: string) => JSON.parse(readFileSync(new URL(`./data/${f}`, import.meta.url), "utf8"));

// Headline blank: the longest plain word that is not the first one (the earlier of two equals), with the headline split around it.
assert.deepEqual(blankOut("RBI keeps repo rate unchanged amid inflation worries"), { title: "RBI keeps repo rate unchanged amid inflation worries", answer: "unchanged", before: "RBI keeps repo rate ", after: " amid inflation worries" });
assert.equal(blankOut("Why it is so"), null);
const items = [1, 2, 3, 4, 5, 6].map((n) => ({ id: `s${n}`, title: `Government announces policy number ${n}`, time: "2026-11-19T10:00:00Z" })).concat([{ id: "old", title: "Something happened somewhere", time: "2026-11-10T10:00:00Z" }]);
assert.equal(headlineOfDay(items, "2026-11-20")?.answer, "announces"); // "Government" is the first word and 10 letters
assert.equal(headlineOfDay(items, "2026-11-20")?.id, headlineOfDay([...items].reverse(), "2026-11-20")?.id); // the order of the input does not matter
assert.equal(headlineOfDay([], "2026-11-20"), null);

// Spelling Bee: every puzzle has a pangram and a workable number of words; the common list is lowercase letters.
const common: string[] = data("common-words.json");
const bees: BeePuzzle[] = data("bee-puzzles.json");
assert.ok(bees.length > 300);
for (const p of bees.slice(0, 40)) {
  const words = beeWords(common, p);
  assert.ok(words.length >= 15 && words.length <= 60, `${p.letters}/${p.center}: ${words.length} words`);
  assert.ok(words.some((w) => isPangram(w, p)), `${p.letters} has a pangram`);
  assert.ok(p.letters.includes(p.center) && p.letters.length === 7 && !p.letters.includes("s"));
}
const demo = { letters: "abcdefg", center: "a" };
assert.deepEqual([beePoints("cage", demo), beePoints("badge", demo), beePoints("abedcfg", demo)], [1, 5, 14]);
assert.deepEqual([beeRank(0, 100), beeRank(50, 100), beeRank(100, 100), beeRank(5, 0)], ["Beginner", "Solid", "Genius", "Beginner"]);

// A made-up puzzle: valid like the stored ones, and the "seen" ids are skipped.
let seed = 7;
const rand = () => ((seed = (seed * 1664525 + 1013904223) % 4294967296) / 4294967296);
for (let n = 0; n < 15; n++) {
  const q = makeBee(common, rand)!;
  const words = beeWords(common, q);
  assert.ok(q && words.length >= 12 && words.length <= 80 && words.some((w) => isPangram(w, q)) && q.letters.includes(q.center), `made bee ${q?.letters}`);
}
const skip = new Set<string>();
for (let n = 0; n < 40; n++) { const q = makeBee(common, rand, skip)!; assert.ok(!skip.has(q.letters + q.center)); skip.add(q.letters + q.center); } // no repeats until the ids are used up

// Word ladder: one-letter steps, and each puzzle's words are in the valid list.
assert.ok(isStep("cold", "cord") && isStep("cold", "bold") && !isStep("cold", "cool") && !isStep("cold", "word") && !isStep("cold", "cold") && !isStep("cold", "colds"));
const ladders: Ladder[] = data("ladders.json");
const valid = new Set<string>(data("words4.json"));
assert.ok(ladders.length > 300 && ladders.every((l) => valid.has(l.from) && valid.has(l.to) && l.par >= 3 && l.par <= 6 && l.from !== l.to));

const four: string[] = data("common4.json");
for (let n = 0; n < 8; n++) {
  const l = makeLadder(four, rand)!;
  // the par is the real shortest distance through the common words
  const dist = new Map([[l.from, 0]]); const queue = [l.from];
  for (let i = 0; i < queue.length; i++) for (const w of four) if (!dist.has(w) && isStep(queue[i], w)) { dist.set(w, dist.get(queue[i])! + 1); queue.push(w); }
  assert.equal(dist.get(l.to), l.par);
  assert.ok(l.par >= 3 && l.par <= 6);
}

// Countries: distances and directions on known pairs; the answer is never a country whose outline we do not draw.
const countries: Country[] = data("countries.json");
const by = (n: string) => countries.find((c) => c.name === n)!;
assert.ok(countries.length > 150 && by("India").d === "" && by("France").d.startsWith("M"));
const km = distanceKm(by("France"), by("Japan"));
assert.ok(km > 9000 && km < 10500, `France to Japan ${km}`);
assert.equal(arrow(by("France"), by("Japan")), "↗️"); // the shortest way runs north-east over Siberia
assert.equal(arrow(by("France"), by("Egypt")), "↘️");
assert.ok(["⬇️", "↘️", "↙️"].includes(arrow(by("France"), by("Brazil"))));
assert.deepEqual([closeness(0), closeness(20000), closeness(10000)], [100, 0, 50]);
for (let d = 1; d <= 40; d++) { const c = countryOfDay(countries, `2026-12-${String(d % 28 + 1).padStart(2, "0")}`); assert.ok(c.d && !["India", "Pakistan", "China"].includes(c.name)); }
assert.ok(Math.abs(by("Russia").w) > 0 && by("Russia").w > by("Russia").h); // Russia is drawn across the date line, not split in two

// Who is it?
assert.equal(maskName("Narendra Modi meets Joe Biden; Modi says yes", "Narendra Modi"), "▮▮▮ ▮▮▮ meets Joe Biden; ▮▮▮ says yes");
assert.equal(maskName("Fed Chair Powell speaks", "Jerome Powell"), "Fed Chair ▮▮▮ speaks");
const ents = [{ name: "B", stories: [1, 2, 3] }, { name: "A", stories: [1, 2, 3] }, { name: "Z", stories: [1] }];
assert.equal(whoOfDay(ents, "2026-01-01")?.name, "A");
assert.equal(whoOfDay([{ name: "Z", stories: [1] }], "2026-01-01"), null);

console.log("games: all checks passed");
