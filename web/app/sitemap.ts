import mapIndex from "@/lib/data/maps/index.json";
import type { MetadataRoute } from "next";
import { getDigest, getGraph, getMonths } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

const DAILY = ["", "/editorials", "/funding", "/underreported", "/week", "/archive"];
const STEADY = ["/quiz", "/flashcards", "/concepts", "/maps", "/maps/news", "/maps/combine", "/ahead", "/graph", "/epaper", "/games", "/games/wordle", "/games/headline", "/games/bee", "/games/ladder", "/games/countries", "/games/who", "/sources", "/privacy", "/terms", "/pro", "/teams", "/month"];

// The pages worth indexing: the fixed pages, the stories of the last week (so a search engine finds each
// story page, newest first), the monthly compilations and the people and places pages. The calls are
// cached, so a crawler hitting the sitemap does not reach the API each time. A failed call leaves its part
// out rather than the whole sitemap.
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const [stories, months, graph] = await Promise.all([
    getDigest({ days: "7", pageSize: 200 }).catch(() => null),
    getMonths().catch(() => [] as string[]),
    getGraph().catch(() => null),
  ]);

  return [
    ...DAILY.map((path) => ({ url: SITE_URL + path, changeFrequency: "daily" as const, priority: path === "" ? 1 : 0.8 })),
    ...(mapIndex as { id: string }[]).map((m) => ({ url: `${SITE_URL}/maps/${m.id}`, changeFrequency: "monthly" as const, priority: 0.4 })),
    ...STEADY.map((path) => ({ url: SITE_URL + path, changeFrequency: "weekly" as const, priority: 0.5 })),
    ...(stories?.items ?? []).map((it) => ({ url: `${SITE_URL}/story/${encodeURIComponent(it.id)}`, lastModified: it.time, changeFrequency: "monthly" as const, priority: 0.6 })),
    ...months.map((m) => ({ url: `${SITE_URL}/month/${encodeURIComponent(m)}`, changeFrequency: "monthly" as const, priority: 0.4 })),
    ...(graph?.entities ?? []).slice(0, 100).map((n) => ({ url: `${SITE_URL}/entity/${encodeURIComponent(n.name)}`, changeFrequency: "weekly" as const, priority: 0.4 })),
  ];
}
