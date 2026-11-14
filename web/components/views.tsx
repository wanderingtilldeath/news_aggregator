import Link from "next/link";
import { getDigest, safeURL, type SourceCount } from "@/lib/api";
import { PAGE_SIZE, catName, categoriesPresent, hostName, hrefFor, pageList, type View } from "@/lib/view";
import { Card, Feature, Lead, Meta, Row } from "./article";

const Empty = ({ text }: { text: string }) => <div className="empty">{text}</div>;

// The marker that makes this a finished edition rather than an endless feed.
export const EditionEnd = () => (
  <p className="done" role="status">
    That is today&rsquo;s edition. You are caught up.
  </p>
);

function Pager({ view, page, pages }: { view: View; page: number; pages: number }) {
  if (pages <= 1) return null;
  const item = (label: string, n: number, disabled: boolean) =>
    disabled ? (
      <span key={label} className="btn disabled" aria-disabled="true">
        {label}
      </span>
    ) : (
      <Link key={label} className="btn" href={hrefFor(view, { category: view.category, source: view.source, q: view.q, page: n })}>
        {label}
      </Link>
    );
  return (
    <nav className="pager" aria-label="Pages">
      {item("← Previous", page - 1, page <= 1)}
      {pageList(page, pages).map((n, i) =>
        n === null ? (
          <span key={`gap${i}`} className="gap">
            …
          </span>
        ) : n === page ? (
          <span key={n} className="btn current" aria-current="page">
            {n}
          </span>
        ) : (
          <Link key={n} className="btn" href={hrefFor(view, { category: view.category, source: view.source, q: view.q, page: n })}>
            {n}
          </Link>
        ),
      )}
      {item("Next →", page + 1, page >= pages)}
    </nav>
  );
}

// Home: one lead story, the latest headlines, then a block per section. A story
// shown once is not repeated further down.
export async function HomeView({ view, sources, now }: { view: View; sources: SourceCount[]; now: number }) {
  const cats = categoriesPresent(sources.map((s) => s.category));
  const [latest, ...sections] = await Promise.all([
    getDigest({ days: view.days, pageSize: 12 }),
    ...cats.map((c) => getDigest({ days: view.days, category: c, pageSize: 8 }).catch(() => null)),
  ]);
  if (!latest.items.length) return <Empty text="No articles yet for this period." />;

  const top = latest.items.find((i) => i.image) ?? latest.items[0];
  const recent = latest.items.filter((i) => i !== top).slice(0, 8);
  const used = new Set([top.id, ...recent.map((i) => i.id)]);

  const blocks = cats.flatMap((c, i) => {
    const items = (sections[i]?.items ?? []).filter((it) => !used.has(it.id)).slice(0, 4);
    items.forEach((it) => used.add(it.id));
    if (!items.length) return [];
    return [
      <section className="block" key={c}>
        <div className="blockhead">
          <h2>{catName(c)}</h2>
          <Link href={hrefFor(view, { category: c })}>More in {catName(c)} →</Link>
        </div>
        <div className="blockbody">
          <Card it={items[0]} now={now} />
          <div className="stack">
            {items.slice(1).map((it) => (
              <Row key={it.id} it={it} now={now} />
            ))}
          </div>
        </div>
      </section>,
    ];
  });

  return (
    <>
      <section className="hero">
        <Lead it={top} now={now} />
        <aside className="latest">
          <h2>Latest</h2>
          <ol>
            {recent.map((it) => {
              const href = safeURL(it.url);
              const title = it.title || it.url;
              return (
                <li key={it.id}>
                  {href ? (
                    <a className="t" href={href} target="_blank" rel="noopener noreferrer">
                      {title}
                    </a>
                  ) : (
                    <span className="t">{title}</span>
                  )}
                  <Meta it={it} now={now} />
                </li>
              );
            })}
          </ol>
        </aside>
      </section>
      {blocks}
      <EditionEnd />
    </>
  );
}

// Section: the chosen category's articles, newest first, with its sources as filters.
export async function SectionView({ view, sources, now }: { view: View; sources: SourceCount[]; now: number }) {
  const body = await getDigest({ days: view.days, category: view.category, source: view.source, page: view.page, pageSize: PAGE_SIZE });
  const mine = sources.filter((s) => s.category === view.category);
  const lastPage = body.page >= body.total_pages;

  return (
    <>
      <div className="sectionhead">
        <h1>{catName(view.category)}</h1>
        <span className="count">{body.total.toLocaleString("en")} articles</span>
      </div>
      {mine.length > 1 && (
        <div className="chips">
          <Link className="chip" aria-pressed={!view.source} href={hrefFor(view, { category: view.category })}>
            All sources
          </Link>
          {mine.map((s) => (
            <Link key={s.source} className="chip" aria-pressed={view.source === s.source.toLowerCase()} href={hrefFor(view, { category: view.category, source: s.source })}>
              {hostName(s.source)} ({s.count})
            </Link>
          ))}
        </div>
      )}
      {!body.items.length ? (
        <Empty text="No articles in this section for this period." />
      ) : (
        <div className="grid">
          {body.items.map((it, i) =>
            i === 0 && body.page === 1 ? <Feature key={it.id} it={it} now={now} /> : <Card key={it.id} it={it} now={now} eager={i < 4} />,
          )}
        </div>
      )}
      <Pager view={view} page={body.page} pages={body.total_pages} />
      {lastPage && body.items.length > 0 && <EditionEnd />}
    </>
  );
}

// Search: a plain list of matching articles.
export async function SearchView({ view, now }: { view: View; now: number }) {
  const body = await getDigest({ days: view.days, q: view.q, page: view.page, pageSize: PAGE_SIZE });
  const lastPage = body.page >= body.total_pages;
  return (
    <>
      <div className="sectionhead">
        <h1>Results for &ldquo;{view.q}&rdquo;</h1>
        <span className="count">{body.total.toLocaleString("en")} articles</span>
      </div>
      {!body.items.length ? (
        <Empty text="No articles match your search in this period." />
      ) : (
        <div className="results">
          {body.items.map((it) => (
            <Row key={it.id} it={it} now={now} withGist />
          ))}
        </div>
      )}
      <Pager view={view} page={body.page} pages={body.total_pages} />
      {lastPage && body.items.length > 0 && <EditionEnd />}
    </>
  );
}
