import type { Concept } from "./api";

// The glossary terms a story mentions, for the "Concepts in this story" cards: whole-word,
// case-insensitive matches in the text, in the order they first appear, at most max. A term
// that is not in the story is never shown, so nothing is stored per story.
export function mentioned(text: string, concepts: Concept[], max = 4): Concept[] {
  const hay = text.toLowerCase();
  const found: { at: number; concept: Concept }[] = [];
  for (const c of concepts) {
    const term = c.term.toLowerCase().trim();
    if (!term) continue;
    const m = new RegExp(`(^|[^a-z0-9])${term.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}($|[^a-z0-9])`).exec(hay);
    if (m) found.push({ at: m.index, concept: c });
  }

  return found
    .sort((a, b) => a.at - b.at)
    .slice(0, max)
    .map((f) => f.concept);
}

// A URL-safe anchor for a term in the glossary page.
export const conceptId = (term: string) => term.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
