// The logic of the five news games, as plain functions with no imports besides wordle.ts, so it runs under plain Node
// for its tests (games.test.ts). The data (word lists, shapes) is passed in; the pages read it from lib/data/.
import { dayNumber } from "./wordle.ts";

const pick = <T>(list: T[], key: string): T => list[((dayNumber(key) % list.length) + list.length) % list.length];

// ---- Headline blank -------------------------------------------------------------------------------------------------
// One word of a real headline is hidden; the reader guesses it the way they guess a Wordle word.
const STOP = new Set("about after again against among because before between could during every first from have into more most other over should since still their there these they this those through under until what when where which while with would your".split(" "));

export type Blank = { id: string; title: string; answer: string; before: string; after: string };

// The longest plain word of 5 to 9 letters that is not a first word or a filler word, with the headline split around it.
export function blankOut(title: string): Omit<Blank, "id"> | null {
  const words = [...title.matchAll(/[A-Za-z]+/g)].filter((m) => m.index! > 0 && m[0].length >= 5 && m[0].length <= 9 && !STOP.has(m[0].toLowerCase()));
  const best = words.sort((a, b) => b[0].length - a[0].length || a.index! - b.index!)[0];
  if (!best) return null;
  return { title, answer: best[0].toLowerCase(), before: title.slice(0, best.index), after: title.slice(best.index! + best[0].length) };
}

// The day's story: from the stories dated the day before (so the pick does not move as the crawl adds stories),
// or from all of them when there are fewer than five; the same for every reader.
export function headlineOfDay(items: { id: string; title: string; time: string }[], key: string): Blank | null {
  const yesterday = new Date(Date.UTC(+key.slice(0, 4), +key.slice(5, 7) - 1, +key.slice(8, 10) - 1)).toISOString().slice(0, 10);
  const day = items.filter((it) => it.time.slice(0, 10) === yesterday);
  const pool = [...(day.length >= 5 ? day : items)].sort((a, b) => a.id.localeCompare(b.id));
  for (let i = 0; i < pool.length; i++) {
    const it = pool[(dayNumber(key) + i) % pool.length];
    const b = blankOut(it.title);
    if (b) return { id: it.id, ...b };
  }
  return null;
}

// ---- Spelling Bee ---------------------------------------------------------------------------------------------------
export type BeePuzzle = { letters: string; center: string };
export const beePuzzleOfDay = (puzzles: BeePuzzle[], key: string) => pick(puzzles, key);

// Words of 4 or more letters that use only the seven letters and include the centre one.
export const beeWords = (common: string[], p: BeePuzzle) => common.filter((w) => w.length >= 4 && w.includes(p.center) && [...w].every((c) => p.letters.includes(c)));
export const isPangram = (w: string, p: BeePuzzle) => [...p.letters].every((c) => w.includes(c));
// 1 point for a four-letter word, its length otherwise, and 7 more for a pangram.
export const beePoints = (w: string, p: BeePuzzle) => (w.length === 4 ? 1 : w.length) + (isPangram(w, p) ? 7 : 0);
export const BEE_RANKS: [string, number][] = [["Beginner", 0], ["Good start", 0.1], ["Moving up", 0.2], ["Good", 0.35], ["Solid", 0.5], ["Nice", 0.7], ["Great", 0.85], ["Genius", 1]];
// The rank for a score out of the most the common words give; "Genius" is every point.
export const beeRank = (points: number, max: number) => [...BEE_RANKS].reverse().find(([, f]) => max > 0 && points / max >= f)?.[0] ?? "Beginner";

// A new puzzle in the browser for practice: a random pangram word gives the seven letters, and a centre letter that
// leaves 12 to 80 words (the "s" is allowed here). About 6,000 puzzles come out of the common-word list.
// `seen` holds the "lettersCentre" ids already played; one that is not in it is preferred, and null means none worked.
export function makeBee(common: string[], rand: () => number, seen: Set<string> = new Set()): BeePuzzle | null {
  const pangrams = common.filter((w) => w.length >= 7 && new Set(w).size === 7);
  for (let tries = 0; tries < 300 && pangrams.length; tries++) {
    const letters = [...new Set(pangrams[Math.floor(rand() * pangrams.length)])].sort().join("");
    const words = common.filter((w) => w.length >= 4 && [...w].every((c) => letters.includes(c)));
    const centres = [...letters].filter((c) => { const n = words.filter((w) => w.includes(c)).length; return n >= 12 && n <= 80 && !seen.has(letters + c); });
    if (centres.length) return { letters, center: centres[Math.floor(rand() * centres.length)] };
  }
  return null;
}

// ---- Word ladder ----------------------------------------------------------------------------------------------------
export type Ladder = { from: string; to: string; par: number };
export const ladderOfDay = (list: Ladder[], key: string) => pick(list, key);
// A legal step changes exactly one letter.
export const isStep = (a: string, b: string) => a.length === b.length && [...a].filter((c, i) => c !== b[i]).length === 1;

// A new ladder in the browser for practice: a random start, then a random word 3 to 6 steps away through the common
// four-letter words (a breadth-first search, about 12 ms). About 280,000 pairs exist.
export function makeLadder(four: string[], rand: () => number): Ladder | null {
  const near = (a: string, b: string) => isStep(a, b);
  const adj = new Map(four.map((w) => [w, four.filter((x) => near(w, x))]));
  for (let tries = 0; tries < 50; tries++) {
    const from = four[Math.floor(rand() * four.length)];
    const dist = new Map([[from, 0]]);
    const queue = [from];
    for (let i = 0; i < queue.length; i++) for (const n of adj.get(queue[i])!) if (!dist.has(n)) { dist.set(n, dist.get(queue[i])! + 1); queue.push(n); }
    const ends = [...dist].filter(([, d]) => d >= 3 && d <= 6);
    if (ends.length) { const [to, par] = ends[Math.floor(rand() * ends.length)]; return { from, to, par }; }
  }
  return null;
}

// ---- Country guess --------------------------------------------------------------------------------------------------
export type Country = { name: string; lat: number; lon: number; area: number; w: number; h: number; d: string };
const R = 6371;
const rad = (d: number) => (d * Math.PI) / 180;
export function distanceKm(a: Country, b: Country) {
  const h = Math.sin(rad(b.lat - a.lat) / 2) ** 2 + Math.cos(rad(a.lat)) * Math.cos(rad(b.lat)) * Math.sin(rad(b.lon - a.lon) / 2) ** 2;
  return Math.round(2 * R * Math.asin(Math.sqrt(h)));
}
const ARROWS = ["⬆️", "↗️", "➡️", "↘️", "⬇️", "↙️", "⬅️", "↖️"];
// The compass arrow from the guess towards the answer.
export function arrow(from: Country, to: Country) {
  const dl = rad(to.lon - from.lon);
  const y = Math.sin(dl) * Math.cos(rad(to.lat));
  const x = Math.cos(rad(from.lat)) * Math.sin(rad(to.lat)) - Math.sin(rad(from.lat)) * Math.cos(rad(to.lat)) * Math.cos(dl);
  return ARROWS[Math.round((((Math.atan2(y, x) * 180) / Math.PI + 360) % 360) / 45) % 8];
}
// How close a guess was, 0 to 100 (half the Earth's circumference is the far end).
export const closeness = (km: number) => Math.max(0, Math.round(100 - (km / 20_000) * 100));
// Only countries that can be drawn and are big enough to recognise are ever the answer.
export const countryOfDay = (list: Country[], key: string) => pick(list.filter((c) => c.d && c.area >= 2), key);

// ---- Who is it? -----------------------------------------------------------------------------------------------------
// One clue is a real headline about the day's name, with that name blanked out; full keeps the headline whole for the
// end of the game.
export type Clue = { id: string; title: string; full: string; source: string };

// Words of a name that are never blanked or guessed on their own: "The Hindu" hides "Hindu", not every "the".
const NAME_STOP = new Set(["the", "and", "for", "of", "new", "de", "la", "al", "bin", "von", "van"]);
const tokensOf = (name: string) => name.split(/\s+/).filter((t) => t.length >= 3 && !NAME_STOP.has(t.toLowerCase()));

// The name (and each word of it) taken out of a headline.
export const maskName = (title: string, name: string) =>
  tokensOf(name).reduce((s, t) => s.replace(new RegExp(`\\b${t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`, "gi"), "▮▮▮"), title);

// Names too broad to make a puzzle: a continent or a kind of body, not a someone. (A country is fine to guess in
// "Country guess"; here, names mentioned in dozens of stories would blank half the paper.)
const BROAD = new Set(["europe", "asia", "africa", "america", "middle east", "west", "east", "north", "south", "government", "police", "court", "army", "state", "centre", "center", "union", "parliament", "opposition", "media", "world", "people", "company", "council", "ministry", "party"]);
const MAX_MENTIONS = 40;

export const goodAnswer = (name: string, count?: number) => name.length >= 3 && name.length <= 40 && !BROAD.has(name.toLowerCase()) && (count === undefined || count <= MAX_MENTIONS);

type WhoStory = { id: string; title: string; source: string };
const MIN_CLUE_WORDS = 5;
const MIN_CLUES = 4;

// The clues for a name: up to six of its stories whose headline actually carries the name (so every clue has a
// blank in it and enough other words to say something about this name), one per distinct headline, in a fixed order.
export function whoClues(stories: WhoStory[], name: string): Clue[] {
  const seen = new Set<string>();
  const out: Clue[] = [];
  for (const s of [...stories].sort((a, b) => a.id.localeCompare(b.id))) {
    const title = maskName(s.title, name);
    // A clue that says nothing once the name is gone ("Inside the new ▮▮▮") is no clue: it needs a few other words.
    if (title === s.title || seen.has(title.toLowerCase()) || title.replace(/▮+/g, " ").split(/\s+/).filter((w) => w.length > 1).length < MIN_CLUE_WORDS) continue;
    seen.add(title.toLowerCase());
    out.push({ id: s.id, title, full: s.title, source: s.source });
    if (out.length === 6) break;
  }

  return out;
}

// The day's name: among the specific names that have at least four clues, in alphabetical order so the list does
// not shuffle.
export function whoOfDay<T extends { name: string; count?: number; stories: WhoStory[] }>(entities: T[], key: string): T | null {
  const pool = entities.filter((e) => goodAnswer(e.name, e.count) && whoClues(e.stories, e.name).length >= MIN_CLUES).sort((a, b) => a.name.localeCompare(b.name));
  return pool.length ? pick(pool, key) : null;
}

const norm = (s: string) =>
  s.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().replace(/[^a-z0-9\s]/g, " ").replace(/\s+/g, " ").trim();

// Whether a typed guess is the answer: the name in any case or punctuation, or one word of a longer name ("Modi" for
// "Narendra Modi"), but never a filler word.
export function isRight(typed: string, answer: string): boolean {
  const t = norm(typed);
  const a = norm(answer);
  if (!t) return false;
  if (t === a) return true;

  return !t.includes(" ") && t.length >= 3 && !NAME_STOP.has(t) && a.split(" ").length > 1 && a.split(" ").includes(t);
}

// The guess as it is kept and shown: a name from the list when it is one (in that spelling), else what was typed,
// trimmed and cut.
export function guessLabel(typed: string, names: string[]): string {
  const t = norm(typed);
  return names.find((n) => norm(n) === t) ?? typed.trim().replace(/\s+/g, " ").slice(0, 40);
}
