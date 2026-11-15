import Link from "next/link";
import { getConcepts, getGraph } from "@/lib/api";
import { conceptId } from "@/lib/concepts";
import { entityHref } from "@/lib/graphlayout";

// A strip on the front page that shows the newer parts of the site where readers will see them: a
// concept of the day and the names in the news. Each card reads the same
// cached data as its own page, and a card with nothing to show is left out, so a quiet day or an API
// that is down just shows fewer cards (or none).
export default async function Discover({ now }: { now: number }) {
  const [concepts, graph] = await Promise.all([getConcepts(), getGraph()]);
  const concept = concepts.length ? concepts[Math.floor(now / 86_400_000) % concepts.length] : null;
  const names = graph?.entities.slice(0, 5) ?? [];
  if (!concept && !names.length) return null;

  return (
    <section className="discover" aria-label="Explore">
      {concept && (
        <div className="dc">
          <h2>Concept of the day</h2>
          <h3>{concept.term}</h3>
          <p>{concept.body}</p>
          <Link className="dc-more" href={`/concepts#${conceptId(concept.term)}`}>
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
