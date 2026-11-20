import Link from "next/link";
import { getConcepts, getGraph } from "@/lib/api";
import { conceptId } from "@/lib/concepts";
import { entityHref } from "@/lib/graphlayout";

// A strip on the front page that shows the newer parts of the site where readers will see them: three
// concepts of the day (enough to fill the card to the height of the names beside it) and the names in the news. Each card reads the same
// cached data as its own page, and a card with nothing to show is left out, so a quiet day or an API
// that is down just shows fewer cards (or none).
export default async function Discover({ now }: { now: number }) {
  const [concepts, graph] = await Promise.all([getConcepts(), getGraph()]);
  // Three consecutive concepts starting at today's, so the set changes every day and none repeats on a page.
  const day = Math.floor(now / 86_400_000);
  const picked = Array.from({ length: Math.min(3, concepts.length) }, (_, i) => concepts[(day + i) % concepts.length]);
  const names = graph?.entities.slice(0, 5) ?? [];
  if (!picked.length && !names.length) return null;

  return (
    <section className="discover" aria-label="Explore">
      {picked.length > 0 && (
        <div className="dc">
          <h2>Concepts of the day</h2>
          <ul className="dc-list">
            {picked.map((c) => (
              <li key={c.term}>
                <h3>
                  <Link href={`/concepts#${conceptId(c.term)}`}>{c.term}</Link>
                </h3>
                <p>{c.body}</p>
              </li>
            ))}
          </ul>
          <Link className="dc-more" href="/concepts">
            All concepts <span aria-hidden="true">→</span>
          </Link>
        </div>
      )}
      {names.length > 0 && (
        <div className="dc">
          <h2>Who is in the news</h2>
          <ol>
            {names.map((n) => (
              <li key={n.name}>
                <Link href={entityHref(n.name)}>{n.name}</Link> <span>{n.count}</span>
              </li>
            ))}
          </ol>
          <Link className="dc-more" href="/graph">
            The whole graph <span aria-hidden="true">→</span>
          </Link>
        </div>
      )}
    </section>
  );
}
