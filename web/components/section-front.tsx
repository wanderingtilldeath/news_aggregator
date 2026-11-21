import type { Item } from "@/lib/api";
import { catName } from "@/lib/view";
import { Blurb, Byline, Headline, Media } from "./article";

// The first page of a section, laid out like a newspaper's section front: a large lead story with two more
// beneath it on the left, a column of picture cards in the middle, and a list of the latest with small pictures on
// the right. It takes the first 15 stories (SECTION_FRONT): 1 lead, 2 beneath it, 3 cards and 9 in the list, which
// is what makes the three columns about as tall as each other. The page lays the rest out as cards below.
//
// "Why it matters" goes with the stories that have room for it: the lead, the two beneath it and the cards (a key
// story, importance 4 or 5, has it highlighted). The compact list is headlines only, so it stays as short as the
// other two columns.
export const SECTION_FRONT = 15;

type Props = {
  items: Item[];
  label: string;
  now: number;
  mode?: string;
  // A line under a story's headline, by story id (the Startups page puts the company, amount and round there).
  tags?: Record<string, string>;
  // The lead's small heading and the list's heading, when "the category" and "Latest in the category" do not fit.
  leadKicker?: string;
  listTitle?: string;
};

export default function SectionFront({ items, label, now, mode, tags, leadKicker, listTitle }: Props) {
  const [lead, ...rest] = items;
  const rows = rest.slice(0, 2);
  const middle = rest.slice(2, 5);
  const side = rest.slice(5, SECTION_FRONT - 1);
  const tag = (it: Item) => (tags?.[it.id] ? <p className="sf-tag">{tags[it.id]}</p> : null);

  return (
    <section className="sf" aria-label={`${label}: the lead stories`}>
      <div className="sf-a">
        <article className="sf-lead">
          <Media it={lead} eager />
          <p className="sf-kick">{leadKicker ?? catName(lead.category || label)}</p>
          <Headline it={lead} as="h2" />
          {tag(lead)}
          <Blurb it={lead} mode={mode} />
          <Byline it={lead} now={now} />
        </article>
        {rows.map((it) => (
          <article key={it.id} className="sf-row">
            <Media it={it} />
            <div>
              <Headline it={it} />
              {tag(it)}
              <Blurb it={it} mode={mode} />
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
              {tag(it)}
              <Blurb it={it} mode={mode} />
              <Byline it={it} now={now} />
            </article>
          ))}
        </div>
      )}

      {side.length > 0 && (
        <aside className="sf-c" aria-label={listTitle ?? `Latest in ${label}`}>
          <h2 className="list-head">{listTitle ?? `Latest in ${label}`}</h2>
          {side.map((it) => (
            <article key={it.id} className="sf-compact">
              <div>
                <Headline it={it} />
                {tag(it)}
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
