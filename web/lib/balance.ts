import type { Item } from "./api";
import { cleanTitle } from "./view";

// The front page has three columns that should end on the same line. Their stories differ in length, so
// the page picks how many stories go in each column from an estimate of how tall each one is. These are
// rough pixel heights for the desktop layout; keep them in step with .text-story, .rank-item,
// .latest-list, .thumb-row and .main-story in app/globals.css. A wrong guess only leaves a little
// empty space under a column (the columns still share one rule height), never a broken layout.

const lines = (text: string, perLine: number) => Math.max(1, Math.ceil(text.length / perLine));

// The text a card shows under its headline: why it matters, else the gist (see Blurb in article.tsx).
const blurbOf = (it: Item, gist: string) => (it.why ? `Why it matters: ${it.why}` : gist);

// Has a chip next to its byline (a kind: opinion, analysis, explainer), which may wrap.
const chipped = (it: Item) => Boolean(it.kind);

export const heightLeft = (it: Item, gist: string) =>
  lines(cleanTitle(it.title || it.url), 24) * 26 + (it.why ? lines(blurbOf(it, gist), 24) * 24 : Math.min(4, lines(gist, 36)) * 20) + (chipped(it) ? 48 : 22) + 24 + 40;

export const heightRank = (it: Item) => lines(cleanTitle(it.title || it.url), 27) * 22 + (chipped(it) ? 48 : 22) + 34;

export const heightLatest = (it: Item) => lines(cleanTitle(it.title || it.url), 26) * 20 + 40;

export const heightRow = (it: Item, gist: string) =>
  Math.max(120, lines(cleanTitle(it.title || it.url), 36) * 24 + lines(blurbOf(it, gist), 46) * 24 + 40) + 44;

export const heightMain = (it: Item, gist: string) =>
  353 + 12 + lines(cleanTitle(it.title || it.url), 28) * 43 + (it.why ? lines(it.why, 60) * 28 : 0) + lines(gist, 66) * 26 + 75;

// fill takes stories from the front of the list while their estimated height still fits the budget
// (a story may overshoot it by up to half its own height), at least one and at most max.
export function fill<T>(items: T[], height: (item: T) => number, budget: number, max = Infinity): T[] {
  const out: T[] = [];
  let used = 0;
  for (const item of items) {
    const h = height(item);
    if (out.length >= max || (out.length > 0 && used + h / 2 > budget)) break;
    out.push(item);
    used += h;
  }

  return out;
}

export const total = <T>(items: T[], height: (item: T) => number) => items.reduce((sum, item) => sum + height(item), 0);
