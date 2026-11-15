import { getDigest } from "@/lib/api";
import { SITE_URL } from "@/lib/site";
import { cleanTitle } from "@/lib/view";

// Text goes into XML, so & < > and quotes are escaped and control characters XML forbids are dropped.
const esc = (s: string) =>
  s
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F]/g, "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");

// The latest stories as RSS 2.0. ?category= narrows it to one section (india, world, tech, ...).
// Every item links to its Postpaper story page, which links on to the publisher.
export async function GET(request: Request) {
  const category = (new URL(request.url).searchParams.get("category") ?? "").trim().toLowerCase();
  const slug = /^[a-z0-9-]{1,24}$/.test(category) ? category : "";
  const body = await getDigest({ days: "2", category: slug || undefined, pageSize: 30 }).catch(() => null);
  if (!body) return new Response("The feed is not available right now.", { status: 502 });

  const items = body.items
    .map((it) => {
      const link = `${SITE_URL}/story/${encodeURIComponent(it.id)}`;
      const text = it.why || it.summary?.[0] || it.description || "";
      return `<item><title>${esc(cleanTitle(it.title || it.url))}</title><link>${esc(link)}</link><guid isPermaLink="true">${esc(link)}</guid><pubDate>${new Date(it.time).toUTCString()}</pubDate><description>${esc(text)}</description></item>`;
    })
    .join("");
  const title = slug ? `Postpaper: ${slug}` : "Postpaper";

  return new Response(
    `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>${esc(title)}</title><link>${SITE_URL}</link><description>The finished daily newspaper: ranked stories and a line on why each matters.</description>${items}</channel></rss>`,
    { headers: { "Content-Type": "application/rss+xml; charset=utf-8", "Cache-Control": "public, s-maxage=600, stale-while-revalidate=3600" } },
  );
}
