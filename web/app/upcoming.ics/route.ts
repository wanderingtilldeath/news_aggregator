import { getEvents } from "@/lib/api";
import { buildICS } from "@/lib/ics";
import { SITE_URL } from "@/lib/site";

// The upcoming events as an iCalendar feed that calendar apps can subscribe to. It reads the same
// cached events the Upcoming page does, and is cached by the CDN for an hour.
export async function GET() {
  const body = buildICS(await getEvents(), SITE_URL, new Date());

  return new Response(body, {
    headers: { "Content-Type": "text/calendar; charset=utf-8", "Content-Disposition": 'inline; filename="postpaper-upcoming.ics"', "Cache-Control": "public, s-maxage=3600, stale-while-revalidate=86400" },
  });
}
