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

// ---- Word ladder ----------------------------------------------------------------------------------------------------
export type Ladder = { from: string; to: string; par: number };
export const ladderOfDay = (list: Ladder[], key: string) => pick(list, key);
// A legal step changes exactly one letter.
export const isStep = (a: string, b: string) => a.length === b.length && [...a].filter((c, i) => c !== b[i]).length === 1;

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
export type Clue = { id: string; title: string; source: string };
// The name (and each word of it) taken out of a headline.
export const maskName = (title: string, name: string) =>
  name.split(/\s+/).filter((t) => t.length >= 3).reduce((s, t) => s.replace(new RegExp(`\\b${t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}\\b`, "gi"), "▮▮▮"), title);

// The day's name: among the names with at least three stories, in alphabetical order so the list does not shuffle.
export function whoOfDay<T extends { name: string; stories: unknown[] }>(entities: T[], key: string): T | null {
  const pool = entities.filter((e) => e.stories.length >= 3).sort((a, b) => a.name.localeCompare(b.name));
  return pool.length ? pick(pool, key) : null;
}
