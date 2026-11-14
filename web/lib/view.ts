// Everything the page shows is determined by the URL (?days=&category=&source=&q=&page=),
// the same query string the Go API and the Worker accept, so links can be shared.

export const DAYS = ["1", "2", "7", "30"] as const;
export const DAY_LABELS: Record<string, string> = { "1": "24h", "2": "2 days", "7": "7 days", "30": "30 days" };
export const DEFAULT_DAYS = "2";
export const PAGE_SIZE = 24;

// Sections in the order a reader expects them; categories a source invents itself
// come after these, and any slug shows as its own name.
const CATEGORY_NAMES: Record<string, string> = { india: "India", world: "World", upsc: "UPSC", tech: "Tech", ai: "AI", other: "Other" };
const CATEGORY_ORDER = Object.keys(CATEGORY_NAMES);
export const catName = (slug: string) => CATEGORY_NAMES[slug] ?? slug;

export function categoriesPresent(slugs: string[]) {
  const present = new Set(slugs);
  return [...CATEGORY_ORDER.filter((c) => present.has(c)), ...[...present].filter((c) => !CATEGORY_NAMES[c]).sort()];
}

// Two editions, each a set of source categories. Keep in step with internal/track.
export const TRACKS: Record<string, { title: string; categories: string[] }> = {
  upsc: { title: "UPSC current affairs", categories: ["india", "world", "upsc"] },
  engineering: { title: "Engineering", categories: ["tech", "ai"] },
};
export const TRACK_IDS = Object.keys(TRACKS);

export const trackOf = (category: string) => TRACK_IDS.find((t) => TRACKS[t].categories.includes(category)) ?? "";

// Reading length: "1" shows the headline and why it matters, "7" the long summary, "" the standard card.
export const MODES = ["", "1", "7"] as const;

// score ranks a story for the lead and the top lists: importance first (an unscored story counts as 2).
export const score = (importance?: number) => importance || 2;

export type View = { days: string; category: string; source: string; q: string; page: number; track: string; mode: string };

type SearchParams = Record<string, string | string[] | undefined>;

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";

export function parseView(sp: SearchParams): View {
  const days = first(sp.days);
  return {
    days: (DAYS as readonly string[]).includes(days) ? days : DEFAULT_DAYS,
    category: first(sp.category).trim().toLowerCase(),
    source: first(sp.source).trim().toLowerCase(),
    q: first(sp.q).trim(),
    page: Math.max(1, parseInt(first(sp.page), 10) || 1),
    track: TRACK_IDS.includes(first(sp.track)) ? first(sp.track) : "",
    mode: first(sp.mode) === "1" || first(sp.mode) === "7" ? first(sp.mode) : "",
  };
}

// hrefFor is the address of a view: the chosen period is kept, the rest comes from over.
export function hrefFor(view: View, over: Partial<View> = {}) {
  // The period, edition and reading length stay; the rest comes from over.
  const s = { days: view.days, category: "", source: "", q: "", page: 1, track: view.track, mode: view.mode, ...over };
  const p = new URLSearchParams();
  if (s.days !== DEFAULT_DAYS) p.set("days", s.days);
  if (s.category) p.set("category", s.category);
  if (s.source) p.set("source", s.source);
  if (s.q) p.set("q", s.q);
  if (s.page > 1) p.set("page", String(s.page));
  if (s.track) p.set("track", s.track);
  if (s.mode) p.set("mode", s.mode);
  const str = p.toString();
  return str ? `/?${str}` : "/";
}

export const hostName = (s: string) => s.replace(/^www\./, "");

export function ago(iso: string, now: number) {
  const mins = Math.max(0, Math.round((now - Date.parse(iso)) / 60000));
  if (mins < 60) return `${mins}m ago`;
  if (mins < 60 * 24) return `${Math.round(mins / 60)}h ago`;
  return `${Math.round(mins / 1440)}d ago`;
}

// pageList gives the page numbers to show: first, last, and a window around the
// current page, with null where numbers are skipped.
export function pageList(page: number, pages: number) {
  const keep = new Set([1, pages, page - 1, page, page + 1]);
  const out: (number | null)[] = [];
  let last = 0;
  for (let n = 1; n <= pages; n++) {
    if (!keep.has(n)) continue;
    if (n - last > 1) out.push(null);
    out.push(n);
    last = n;
  }
  return out;
}
