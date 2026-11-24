import Link from "next/link";
import { connection } from "next/server";
import { getMapNews } from "@/lib/map-news-data";
import { cleanTitle } from "@/lib/view";

// Under a map: the places on it that this week's stories name, each with its newest stories. Left out when none are named
// or the news cannot be reached, so a map page never shows an error for it.
export default async function MapNewsStrip({ title }: { title: string }) {
  // Asked at request time, never while the map pages are built: a build with no API behind it (or a slow one) must still pass.
  await connection();
  const places = ((await getMapNews()) ?? []).filter((p) => p.on.includes(title)).slice(0, 6);
  if (!places.length) return null;

  return (
    <section className="mp-news" aria-label={`${title} in the news`}>
      <h2>On this map, in the news this week</h2>
      <ul>
        {places.map((p) => (
          <li key={p.key}>
            <strong>{p.name}</strong>
            <span>
              {p.count} {p.count === 1 ? "story" : "stories"}
            </span>
            <Link href={`/story/${encodeURIComponent(p.stories[0].id)}`}>{cleanTitle(p.stories[0].title)}</Link>
          </li>
        ))}
      </ul>
      <p className="fine-print">
        <Link href="/maps/news">See every place in the news</Link>
      </p>
    </section>
  );
}
