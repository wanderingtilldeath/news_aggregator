import { cacheLife } from "next/cache";
import { safeURL } from "./safe";

export { safeURL };

// Shapes returned by the Go API (internal/store.Item, internal/api digest handlers).
export type Item = {
  id: string;
  url: string;
  title: string;
  description?: string;
  snippet?: string;
  source: string;
  image?: string;
  summary?: string[] | null;
  is_article: boolean;
  time: string;
  category?: string;
  // From the enrichment pass; absent until it has run.
  why?: string;
  tags?: string[];
  importance?: number;
  // The 7-minute summary; only present when the request asked for detail.
  long?: string;
  // Exam relevance of a UPSC story: Prelims, Mains, Essay, Ethics, each 0 to 3. The full lens
  // (exam) comes only with a single story. source_type is the kind of publisher.
  rel?: number[];
  exam?: ExamLens;
  source_type?: string;
  // "opinion", "analysis" or "explainer" from the URL path; absent for a report.
  kind?: string;
  // How many sources cover the same event (absent when alone). also lists the other sources'
  // stories and comes only with a single story.
  coverage?: number;
  // The opening of the article in whole sentences; comes only with a single story.
  lead?: string;
  also?: { id: string; title: string; source: string; source_type?: string }[];
};

export type ExamLens = {
  prelims: number;
  mains: number;
  essay: number;
  ethics: number;
  dimensions?: string[];
  for?: string[];
  against?: string[];
  facts?: string[];
  skeleton?: string[];
};

export type QuizQuestion = { question: string; options: string[]; answer: number; explanation: string; url?: string };
export type Quiz = { day: string; track: string; questions: QuizQuestion[] };

// One flashcard of the deck: id is stable per card (story and position), story the story it came from.
export type DeckCard = { id: string; q: string; a: string; story: string; title: string };

// A month's compilation of one edition: its most important stories by syllabus area.
export type CompItem = { id: string; url: string; title: string; source: string; why?: string; bullets?: string[]; tags?: string[]; importance: number; time: string };
export type CompArea = { name: string; items: CompItem[] };
export type Compilation = { month: string; track: string; areas: CompArea[]; built: string };

export type Intro = { day: string; track: string; text: string };

// One site Postpaper reads, from the public sources.json the crawl job publishes with the snapshot.
// policy is "restricts" when the site's robots.txt asks AI users to keep out.
export type PublicSource = { host: string; names: string[]; categories: string[]; note: string; policy: "restricts" | "open" };

export type DigestPage = {
  items: Item[];
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
};

export type SourceCount = { source: string; category: string; count: number };

// The Cloudflare Worker in front of the Go API (or a local crawlerx). It is read on
// the server only, so the browser never calls it and no CORS is needed.
export const API_BASE = (process.env.API_BASE_URL ?? "http://localhost:8080").replace(/\/+$/, "");

// The 1 and 2 day views also list older articles, newest first, so a quiet day or
// weekend never leaves a section empty. Same rule as the old static page.
export const fetchDays = (days: string) => (days === "1" || days === "2" ? "7" : days);

// One cached GET. The key is the path plus query string, so each distinct view is cached on its
// own: fresh for 30 minutes, served stale for up to a day while it refreshes. The data changes
// only when the crawl runs (a few times a day), and every API call costs the free tiers behind it.
async function getJSON<T>(pathAndQuery: string, nullOn404 = false): Promise<T> {
  "use cache";
  cacheLife({ stale: 300, revalidate: 1800, expire: 86400 });
  // Production hides server error text from the page (React error #441), so say
  // what failed in the logs, where the real cause can be read.
  let res: Response;
  try {
    res = await fetch(API_BASE + pathAndQuery);
  } catch (e) {
    console.error(`API request failed: ${API_BASE}${pathAndQuery} (is API_BASE_URL set?)`, e);
    throw e;
  }
  if (nullOn404 && res.status === 404) return null as T;
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    console.error(`API returned ${res.status} for ${API_BASE}${pathAndQuery}`);
    throw new Error(body.error ?? `API returned ${res.status}`);
  }
  return body as T;
}

function qs(params: Record<string, string | number | undefined>) {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") p.set(k, String(v));
  return p.toString();
}

export function getDigest(params: {
  days: string;
  category?: string;
  source?: string;
  q?: string;
  page?: number;
  pageSize: number;
  // detail adds the 7-minute summaries; day (YYYY-MM-DD) asks for one edition day.
  // Both are answered by the origin, not the Worker snapshot.
  detail?: boolean;
  day?: string;
}) {
  const { pageSize, days, detail, day, ...rest } = params;
  const extra = { detail: detail ? "1" : undefined, day };
  return getJSON<DigestPage>(`/digest?${qs({ ...rest, ...extra, days: day ? undefined : fetchDays(days), page_size: pageSize })}`);
}

// One story by id (the story page); null when it is gone (pages are kept about 60 days).
export function getStory(id: string) {
  return getJSON<Item | null>(`/story/${encodeURIComponent(id)}`, true);
}

// The quiz of a track for an edition day, or the latest when day is omitted; null when none exists.
export function getQuiz(track: string, day?: string) {
  return getJSON<Quiz | null>(`/quiz?${qs({ track, day })}`, true);
}

// The "Today in 60 seconds" paragraph of a track, or the latest; null when none was written yet.
export function getIntro(track: string, day?: string) {
  return getJSON<Intro | null>(`/intro?${qs({ track, day })}`, true);
}

export type ProStatus = { active: boolean; until: string };

// A Pro member's link: null when it is not valid, "error" when the API could not be asked. It is
// not cached, because a payment can make a member active at any moment.
export async function getProStatus(token: string): Promise<ProStatus | null | "error"> {
  try {
    const res = await fetch(`${API_BASE}/pro?token=${encodeURIComponent(token)}`, { cache: "no-store", signal: AbortSignal.timeout(15_000) });
    if (res.status === 404 || res.status === 400) return null;
    if (!res.ok) return "error";
    return (await res.json()) as ProStatus;
  } catch {
    return "error";
  }
}

// The flashcard deck (the last two weeks of Prelims cards, most important stories first); null when
// the API could not be reached.
export async function getCards() {
  const body = await getJSON<{ cards: DeckCard[] }>("/cards?days=14&limit=200").catch(() => null);
  return body?.cards ?? null;
}

// The months that have a compilation, newest first (empty when none yet or the API is unreachable).
export async function getMonths() {
  const body = await getJSON<{ months: string[] }>("/months").catch(() => null);
  return body?.months ?? [];
}

// One month of one edition; null when there is none.
export function getMonth(month: string, track: string) {
  return getJSON<Compilation | null>(`/month?${qs({ month, track })}`, true).catch(() => null);
}

// The sites Postpaper reads; an empty list when the file is not published yet.
export async function getPublicSources() {
  const body = await getJSON<{ sources: PublicSource[] }>("/sources.json").catch(() => null);
  return body?.sources ?? [];
}

export async function getSources(days: string, q: string) {
  const body = await getJSON<{ sources: SourceCount[] }>(`/digest/sources?${qs({ days: fetchDays(days), q })}`);
  return body.sources;
}

export const imageSrc = (it: Item) => safeURL(it.image, API_BASE + "/");
