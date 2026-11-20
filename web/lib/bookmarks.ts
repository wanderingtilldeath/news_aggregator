// The reader's bookmarks: stories saved to read later, kept in their own browser (localStorage). Nothing is
// sent anywhere. This file has no server code, so client components and Node tests can import it.

export type Bookmark = { id: string; title: string; source: string; time: string; at: number };

export const BOOKMARKS_KEY = "pp-bookmarks";
export const MAX_BOOKMARKS = 300;

export function parseBookmarks(raw: string): Bookmark[] {
  try {
    const list = JSON.parse(raw || "[]");
    return Array.isArray(list) ? list.filter((b) => b && typeof b.id === "string" && typeof b.title === "string") : [];
  } catch {
    return [];
  }
}

// toggled returns the list with this story added at the front, or removed when it is already saved.
export function toggled(list: Bookmark[], mark: Bookmark): Bookmark[] {
  return list.some((b) => b.id === mark.id) ? list.filter((b) => b.id !== mark.id) : [mark, ...list].slice(0, MAX_BOOKMARKS);
}
