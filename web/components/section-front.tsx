import type { Item } from "@/lib/api";
import { catName } from "@/lib/view";
import { Blurb, Byline, Headline, Media } from "./article";

// The first page of a section, laid out like a newspaper's section front: a large lead story with two more
// beneath it on the left, a column of picture cards in the middle, and a list of the latest with small pictures on
// the right. It takes the first 12 stories (SECTION_FRONT); the page lays the rest out as cards below.
export const SECTION_FRONT = 12;

export default function SectionFront({ items, label, now, mode }: { items: Item[]; label: string; now: number; mode?: string }) {
  const [lead, ...rest] = items;
  const rows = rest.slice(0, 2);
  const middle = rest.slice(2, 5);
  const side = rest.slice(5, SECTION_FRONT - 1);

  return (
    <section className="sf" aria-label={`${label}: the lead stories`}>
      <div className="sf-a">
        <article className="sf-lead">
          <Media it={lead} eager />
          <p className="sf-kick">{catName(lead.category || label)}</p>
          <Headline it={lead} as="h2" />
          <Blurb it={lead} mode={mode} />
          <Byline it={lead} now={now} />
        </article>
        {rows.map((it) => (
          <article key={it.id} className="sf-row">
            <Media it={it} />
            <div>
              <Headline it={it} />
              <Byline it={it} now={now} />
            </div>
          </article>
        ))}
      </div>

      {middle.length > 0 && (
        <div className="sf-b">
          {middle.map((it) => (
            <article key={it.id} className="sf-card">
              <Media it={it} />
              <Headline it={it} />
              <Byline it={it} now={now} />
            </article>
          ))}
        </div>
      )}

      {side.length > 0 && (
        <aside className="sf-c" aria-label={`Latest in ${label}`}>
          <h2 className="list-head">Latest in {label}</h2>
          {side.map((it) => (
            <article key={it.id} className="sf-compact">
              <div>
                <Headline it={it} />
                <Byline it={it} now={now} />
              </div>
              <Media it={it} />
            </article>
          ))}
        </aside>
      )}
    </section>
  );
}
