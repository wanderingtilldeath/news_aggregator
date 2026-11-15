// The reader's notebook: sentences saved from story pages, kept in their own browser (localStorage).
// Nothing is sent anywhere. This file has no server code, so client components can import it.

export type Note = { id: string; text: string; story: string; title: string; at: number };

export const NOTEBOOK_KEY = "pp-notebook";
export const MAX_NOTES = 300;

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

// toggled returns the notes with this one added at the front, or removed when it is already saved.
export function toggled(notes: Note[], note: Note): Note[] {
  return notes.some((n) => n.id === note.id) ? notes.filter((n) => n.id !== note.id) : [note, ...notes].slice(0, MAX_NOTES);
}
