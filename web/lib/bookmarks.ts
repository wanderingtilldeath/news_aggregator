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

// filterBookmarks keeps the stories whose title or publisher has every word of the query (case ignored).
export function filterBookmarks(list: Bookmark[], query: string): Bookmark[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  return list.filter((b) => {
    const hay = `${b.title} ${b.source}`.toLowerCase();
    return words.every((w) => hay.includes(w));
  });
}

export type SavedGroup = { label: "Today" | "Yesterday" | "Earlier this week" | "Earlier"; items: Bookmark[] };

// groupBySaved sorts the stories into when they were saved, counted in days of the reader's own clock: today,
// yesterday, the five days before that, and earlier. The order inside a group is the order given (newest saved
// first), and a group with nothing in it is left out.
export function groupBySaved(list: Bookmark[], now: number): SavedGroup[] {
  const day = (t: number) => {
    const d = new Date(t);
    return Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()) / 86400000;
  };
  const today = day(now);
  const groups: SavedGroup[] = [
    { label: "Today", items: [] },
    { label: "Yesterday", items: [] },
    { label: "Earlier this week", items: [] },
    { label: "Earlier", items: [] },
  ];
  for (const b of list) {
    const age = today - day(b.at);
    groups[age <= 0 ? 0 : age === 1 ? 1 : age <= 6 ? 2 : 3].items.push(b);
  }

  return groups.filter((g) => g.items.length > 0);
}
