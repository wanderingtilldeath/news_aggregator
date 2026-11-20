import type { MetadataRoute } from "next";
import { getDigest } from "@/lib/api";
import { SITE_URL } from "@/lib/site";

// The fixed pages and the stories of the last week, so a search engine finds each story page. The
// digest call is cached, so a crawler hitting the sitemap does not reach the API each time.
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const fixed = ["", "/quiz", "/games", "/games/wordle", "/games/headline", "/games/bee", "/games/ladder", "/games/countries", "/games/who", "/archive", "/week", "/editorials"].map((path) => ({ url: SITE_URL + path, changeFrequency: "daily" as const }));
  const stories = await getDigest({ days: "7", pageSize: 200 }).catch(() => null);

  return [
    ...fixed,
    ...(stories?.items ?? []).map((it) => ({ url: `${SITE_URL}/story/${encodeURIComponent(it.id)}`, lastModified: it.time })),
  ];
}
