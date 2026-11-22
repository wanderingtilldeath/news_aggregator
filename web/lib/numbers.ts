// The figures in a story, pulled out with rules (no model): amounts of money, percentages and counted things, each with
// the few words that follow it, so a chip reads "5.5% repo rate" and not a bare number. Read from the text the reader
// already sees (the long summary, else the gist, else the lead), never from a source the page does not show.

export type KeyNumber = { figure: string; label: string };

const MONEY = String.raw`(?:₹|Rs\.?|INR|US\$|\$|€|£)\s?\d[\d,]*(?:\.\d+)?(?:\s?(?:lakh\s+crore|crore|lakh|million|billion|trillion|thousand|Cr|Mn|Bn|bn|mn|k|K)\b)?`;
const PERCENT = String.raw`\d+(?:\.\d+)?\s?(?:%|per\s?cent\b|percent\b)`;
const COUNT = String.raw`\b\d[\d,]*(?:\.\d+)?(?:\s?(?:crore|lakh|million|billion))?\s(?:people|jobs|students|farmers|workers|voters|seats|votes|cases|deaths|vehicles|units|houses|homes|villages|districts|schools|hospitals|MW|GW|km|tonnes|tons|hectares|acres)\b`;
const FIGURE = new RegExp(`${MONEY}|${PERCENT}|${COUNT}`, "gi");

// Words that cannot end a label: it would read as cut off.
const TRAILING = new Set(["a", "an", "the", "of", "in", "to", "for", "and", "or", "by", "on", "at", "from", "with", "as", "is", "was", "that", "which", "its", "their", "his", "her"]);
const MAX_CHIPS = 4;
const MAX_LABEL_WORDS = 4;

function labelAfter(text: string, end: number): string {
  // Up to a comma, full stop, bracket or other figure, then at most a few words.
  const rest = text.slice(end).replace(/^[\s]+/, "").split(/[.,;:()\n]|\s[-–—]\s/)[0] ?? "";
  const words = rest.split(/\s+/).filter(Boolean);
  const kept: string[] = [];
  for (const w of words) {
    if (/\d/.test(w) || kept.length === MAX_LABEL_WORDS) break;
    kept.push(w);
  }
  while (kept.length && TRAILING.has(kept[kept.length - 1].toLowerCase())) kept.pop();

  return kept.join(" ");
}

// keyNumbers lists up to four distinct figures in order of appearance. A figure that repeats (the same text, whatever
// the case) counts once, and a bare "5" is never a figure: it needs a currency, a percent sign or a counted unit.
export function keyNumbers(text: string): KeyNumber[] {
  const out: KeyNumber[] = [];
  const seen = new Set<string>();
  for (const m of text.matchAll(FIGURE)) {
    const figure = m[0].replace(/\s+/g, " ").trim();
    const key = figure.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    // A counted figure already holds its unit ("12 lakh people"): its label would repeat it.
    const counted = new RegExp(`^${COUNT}$`, "i").test(figure);
    out.push({ figure, label: counted ? "" : labelAfter(text, (m.index ?? 0) + m[0].length) });
    if (out.length === MAX_CHIPS) break;
  }

  return out;
}
