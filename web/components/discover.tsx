import Link from "next/link";
import { getConcepts, getEvents, getGraph } from "@/lib/api";
import { conceptId } from "@/lib/concepts";
import { entityHref } from "@/lib/graphlayout";

const shortDay = (day: string) => new Date(`${day}T12:00:00Z`).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "UTC" });

// A strip on the front page that shows the newer parts of the site where readers will see them: the
// next dates coming up, a concept of the day and the names in the news. Each card reads the same
// cached data as its own page, and a card with nothing to show is left out, so a quiet day or an API
// that is down just shows fewer cards (or none).
export default async function Discover({ now }: { now: number }) {
  const [events, concepts, graph] = await Promise.all([getEvents(), getConcepts(), getGraph()]);
  const next = events.slice(0, 3);
  const concept = concepts.length ? concepts[Math.floor(now / 86_400_000) % concepts.length] : null;
  const names = graph?.entities.slice(0, 5) ?? [];
  if (!next.length && !concept && !names.length) return null;

  return (
    <section className="discover" aria-label="Explore">
      {next.length > 0 && (
        <div className="dc">
          <h2>Coming up</h2>
          <ul>
            {next.map((e) => (
              <li key={`${e.story}-${e.text}`}>
                <b>{shortDay(e.day)}</b> {e.text}
              </li>
            ))}
          </ul>
          <Link className="dc-more" href="/upcoming">
            All upcoming dates <span aria-hidden="true">→</span>
          </Link>
        </div>
      )}
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
