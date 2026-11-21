import type { MetadataRoute } from "next";
import { SITE_URL } from "@/lib/site";

// Everything is open to search engines except the pages that carry a personal link or only make sense to one
// reader (manage, unsubscribe and confirm links, the analytics page, a member's printable editions, a
// reader's own saved lists) and the search results, which are endless near-duplicates of the front page.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: { userAgent: "*", allow: "/", disallow: ["/manage", "/unsubscribe", "/confirm", "/analytics", "/admin", "/pro/edition", "/teams/stop", "/bookmarks", "/notebook", "/*?q="] },
    sitemap: `${SITE_URL}/sitemap.xml`,
  };
}
