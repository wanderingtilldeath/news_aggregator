import { cacheLife } from "next/cache";

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
};

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

// One cached GET. The key is the path plus query string, so each distinct view is
// cached on its own for 5 minutes (the Worker snapshot only changes once a day).
async function getJSON<T>(pathAndQuery: string): Promise<T> {
  "use cache";
  cacheLife({ stale: 60, revalidate: 300, expire: 3600 });
  // Production hides server error text from the page (React error #441), so say
  // what failed in the logs, where the real cause can be read.
  let res: Response;
  try {
    res = await fetch(API_BASE + pathAndQuery);
  } catch (e) {
    console.error(`API request failed: ${API_BASE}${pathAndQuery} (is API_BASE_URL set?)`, e);
    throw e;
  }
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
}) {
  const { pageSize, days, ...rest } = params;
  return getJSON<DigestPage>(`/digest?${qs({ ...rest, days: fetchDays(days), page_size: pageSize })}`);
}

export async function getSources(days: string, q: string) {
  const body = await getJSON<{ sources: SourceCount[] }>(`/digest/sources?${qs({ days: fetchDays(days), q })}`);
  return body.sources;
}

// Only absolute http(s) URLs reach href/src, so a hostile page cannot smuggle in a
// javascript: link. Relative URLs are resolved against the API (used for /img/{id}).
export function safeURL(u: string | undefined, base?: string): string {
  if (!u) return "";
  try {
    const x = new URL(u, base);
    return x.protocol === "http:" || x.protocol === "https:" ? x.href : "";
  } catch {
    return "";
  }
}

export const imageSrc = (it: Item) => safeURL(it.image, API_BASE + "/");
