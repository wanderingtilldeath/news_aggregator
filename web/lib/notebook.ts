// The reader's notebook: sentences saved from story pages, kept in their own browser (localStorage).
// Nothing is sent anywhere. This file has no server code, so client components can import it.

// note and tags are the member's own words and labels, a Pro feature; a free notebook never has them.
export type Note = { id: string; text: string; story: string; title: string; at: number; note?: string; tags?: string[] };

// A note the reader typed, not saved from a story, belongs to this pseudo story. A real story id is the hash of its
// address, so it can never be this word; the server accepts it like any other story id.
export const OWN_STORY = "own";
export const OWN_TITLE = "My own notes";
// The longest saved sentence or own note (the server's limit as well).
export const MAX_TEXT = 2000;

export const NOTEBOOK_KEY = "pp-notebook";
// A free notebook holds 300 sentences; a Pro notebook, which is also kept on the server, holds 10,000.
export const MAX_NOTES = 300;
export const PRO_MAX_NOTES = 10000;
// What this browser remembers about the account, none of it secret (the session is an httpOnly cookie): the username
// the bar shows, whether the notepad syncs (a Pro account is logged in, "1"), which account the notes were last
// synced under (so the first sync uploads what this browser already held, once), and whether a change failed to
// send (so the next sync uploads what the server is missing).
export const ACCOUNT_KEY = "pp-account";
export const SYNC_KEY = "pp-sync";
// A hint, not a permission: this browser last saw an admin account, so the bar shows a link to /admin. /admin checks for real.
export const ADMIN_HINT_KEY = "pp-admin-hint";
export const PRO_SYNCED_KEY = "pp-pro-synced";
export const PRO_DIRTY_KEY = "pp-pro-dirty";

export function parseNotes(raw: string): Note[] {
  try {
    const list = JSON.parse(raw || "[]");
    return Array.isArray(list) ? list.filter((n) => n && typeof n.id === "string" && typeof n.text === "string") : [];
  } catch {
    return [];
  }
}

// A note's id is the story plus a hash of its text, so saving the same sentence twice is one note.
export function noteId(story: string, text: string) {
  let h = 5381;
  for (let i = 0; i < text.length; i++) h = ((h * 33) ^ text.charCodeAt(i)) >>> 0;
  return `${story}:${h.toString(36)}`;
}

// ownNote makes a note from what the reader typed, or null when it is empty. The id carries the time and a random part
// (passed in, so the function stays plain), so two notes never share one. Line breaks are kept; the length is cut to
// MAX_TEXT.
export function ownNote(input: string, now: number, rand: string): Note | null {
  const text = input.trim().slice(0, MAX_TEXT).trim();
  if (!text) return null;

  return { id: `${OWN_STORY}:${now.toString(36)}-${rand.replace(/[^a-z0-9]/gi, "").slice(0, 8)}`, text, story: OWN_STORY, title: OWN_TITLE, at: now };
}

// edited returns the notes with the text of one replaced, keeping its id (so the star on its story stays lit), or null
// when the new text is empty or the same, or the note is not there. A sentence cut from a story can be reworded; the
// story link and title stay.
export function edited(notes: Note[], id: string, text: string): Note[] | null {
  const next = text.trim().slice(0, MAX_TEXT).trim();
  const at = notes.findIndex((n) => n.id === id);
  if (!next || at < 0 || notes[at].text === next) return null;

  return notes.map((n, i) => (i === at ? { ...n, text: next } : n));
}

// toggled returns the notes with this one added at the front, or removed when it is already saved.
export function toggled(notes: Note[], note: Note, max = MAX_NOTES): Note[] {
  return notes.some((n) => n.id === note.id) ? notes.filter((n) => n.id !== note.id) : [note, ...notes].slice(0, max);
}

// cleanTag turns what a reader typed into a short lower-case word, the same rule the server applies.
export function cleanTag(t: string) {
  const words = t.toLowerCase().replace(/[^a-z0-9\s_-]/g, "").trim().split(/[\s_-]+/).filter(Boolean);
  return words.join("-").slice(0, 24).replace(/-+$/, "");
}

// parseTags reads a comma separated list of tags: cleaned, without repeats, at most ten.
export function parseTags(input: string): string[] {
  const out: string[] = [];
  for (const part of input.split(",")) {
    const t = cleanTag(part);
    if (t && !out.includes(t) && out.length < 10) out.push(t);
  }
  return out;
}

// allTags lists the tags in use, the most used first.
export function allTags(notes: Note[]): string[] {
  const counts = new Map<string, number>();
  for (const n of notes) for (const t of n.tags ?? []) counts.set(t, (counts.get(t) ?? 0) + 1);
  return [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).map(([t]) => t);
}

// filterNotes keeps the notes that mention every word of the query (in the sentence, the member's note, the story
// title or a tag) and, when a tag is chosen, carry it.
export function filterNotes(notes: Note[], query: string, tag: string): Note[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return notes.filter((n) => {
    if (tag && !(n.tags ?? []).includes(tag)) return false;
    const hay = [n.text, n.note ?? "", n.title, ...(n.tags ?? [])].join(" ").toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

// tokenFromInput accepts what a member pastes: their Pro link (…/pro/edition?token=…) or the token alone.
export function tokenFromInput(input: string): string {
  const text = input.trim();
  const m = /[?&]token=([^&#\s]+)/.exec(text);
  const token = m ? decodeURIComponent(m[1]) : text;
  return /^[A-Za-z0-9_-]{16,128}$/.test(token) ? token : "";
}

// toMarkdown writes the notes as a Markdown file, one heading per story (linked to its Postpaper page)
// with each saved sentence as a bullet. Notion, Obsidian and most note apps import Markdown.
export function toMarkdown(notes: Note[], origin: string): string {
  const stories = new Map<string, { title: string; notes: Note[] }>();
  for (const n of notes) {
    if (!stories.has(n.story)) stories.set(n.story, { title: n.title, notes: [] });
    stories.get(n.story)!.notes.push(n);
  }
  const sections = [...stories.entries()].map(([story, g]) => {
    const title = g.title.replace(/([\\[\]])/g, "\\$1");
    const bullet = (n: Note) => {
      const own = n.note?.trim() ? `\n  - My note: ${n.note.replace(/\s+/g, " ")}` : "";
      const tags = n.tags?.length ? ` ${n.tags.map((t) => `#${t}`).join(" ")}` : "";
      return `- ${n.text.replace(/\s+/g, " ")}${tags}${own}`;
    };
    // The reader's own notes have no story page to link to.
    const heading = story === OWN_STORY ? `## ${title}` : `## [${title}](${origin}/story/${encodeURIComponent(story)})`;
    return `${heading}\n\n${g.notes.map(bullet).join("\n")}`;
  });
  return `# My Postpaper notepad\n\n${sections.join("\n\n")}\n`;
}
