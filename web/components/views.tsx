import Link from "next/link";
import { getDigest, safeURL, type Item, type SourceCount } from "@/lib/api";
import { PAGE_SIZE, TRACKS, catName, cleanTitle, categoriesPresent, hostName, hrefFor, pageList, score, type View } from "@/lib/view";
import { Card, Feature, Lead, Meta, Row } from "./article";
import Banner from "./hero";

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

// words counts the text a reader sees for a story, to estimate how long the briefing takes to read.
const words = (it: Item) => [it.title, it.why, ...(it.summary ?? [])].join(" ").split(/\s+/).filter(Boolean).length;

// Home: the Daily Briefing (the most important story with the six next to it), the top stories
// by importance, then a row of stories per section. A story shown once is not repeated.
export async function HomeView({ view, sources, now }: { view: View; sources: SourceCount[]; now: number }) {
  const present = categoriesPresent(sources.map((s) => s.category));
  // An edition (track) shows only its own categories.
  const cats = view.track ? present.filter((c) => TRACKS[view.track].categories.includes(c)) : present;
  const detail = view.mode === "7";
  const [latest, ...sections] = await Promise.all([
    view.track ? Promise.resolve(null) : getDigest({ days: view.days, pageSize: 24, detail }),
    ...cats.map((c) => getDigest({ days: view.days, category: c, pageSize: 8, detail }).catch(() => null)),
  ]);

  // The pool the briefing and top stories come from: the newest stories overall, or, inside an
  // edition, everything its categories returned.
  const pool: Item[] = latest ? latest.items : sections.flatMap((s) => s?.items ?? []).sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
  if (!pool.length) return <Empty text="No articles yet for this period." />;

  const ranked = [...pool].sort((a, b) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time));
  const lead = ranked.find((i) => i.image) ?? ranked[0];
  const brief = ranked.filter((i) => i !== lead).slice(0, 6);
  const top = ranked.filter((i) => i !== lead && !brief.includes(i)).slice(0, 6);
  const used = new Set([lead, ...brief, ...top].map((i) => i.id));

  const shown = [lead, ...brief];
  const briefSources = new Set(shown.map((i) => i.source)).size;
  const minutes = Math.max(1, Math.round(shown.reduce((n, i) => n + words(i), 0) / 200));

  const blocks = cats.flatMap((c, i) => {
    const items = (sections[i]?.items ?? []).filter((it) => !used.has(it.id)).slice(0, 3);
    items.forEach((it) => used.add(it.id));
    if (!items.length) return [];
    return [
      <section key={c}>
        <div className="sec-head">
          <h2>{catName(c)}</h2>
          <Link href={hrefFor(view, { category: c })}>See all {catName(c)} →</Link>
        </div>
        <div className="cards">
          {items.map((it) => (
            <Card key={it.id} it={it} now={now} mode={view.mode} />
          ))}
        </div>
      </section>,
    ];
  });

  const total = sources.reduce((n, s) => n + s.count, 0);
  const hosts = new Set(sources.map((s) => s.source)).size;
  const dateLabel = new Date(now).toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Kolkata" });

  return (
    <>
      <section className="intro" aria-label="Postpaper">
        <div>
          <p className="eyebrow">{dateLabel}</p>
          <h1>{view.track ? `${TRACKS[view.track].title}, today.` : "What matters today, in one minute or seven."}</h1>
          <p className="sub">Ranked stories for UPSC aspirants and engineers, each with a line on why it matters. Read it, take the quiz, and be done.</p>
          <div className="cta">
            <Link className="btn primary" href="/#subscribe">
              Get it by email
            </Link>
            <Link className="btn" href="/quiz">
              Take today&rsquo;s quiz
            </Link>
          </div>
        </div>
        <ul className="facts">
          <li>
            <b className="num">{total.toLocaleString("en")}</b>
            <span>stories collected</span>
          </li>
          <li>
            <b className="num">{hosts}</b>
            <span>sources read</span>
          </li>
          <li>
            <b className="num">{Object.keys(TRACKS).length}</b>
            <span>editions</span>
          </li>
        </ul>
      </section>

      <section className="briefing" aria-label="Daily briefing">
        <Lead it={lead} now={now} mode={view.mode} />
        <aside className="brief-panel">
          <span className="eyebrow">Daily briefing</span>
          <h2>Also today</h2>
          <p className="stats">
            <span>
              <b>{shown.length}</b> stories
            </span>
            <span>
              <b>{briefSources}</b> sources
            </span>
            <span>
              <b>{minutes}</b> min read
            </span>
          </p>
          <ol className="brief-list">
            {brief.map((it) => {
              const href = safeURL(it.url);
              const title = cleanTitle(it.title || it.url);
              return (
                <li key={it.id}>
                  {href ? (
                    <a className="t" href={href} target="_blank" rel="noopener noreferrer">
                      {title}
                    </a>
                  ) : (
                    <span className="t">{title}</span>
                  )}
                  {it.why && <p className="w">{it.why}</p>}
                  <Meta it={it} now={now} />
                </li>
              );
            })}
          </ol>
        </aside>
      </section>

      {top.length > 0 && (
        <section>
          <div className="sec-head">
            <h2>Top stories</h2>
            <span className="sub">Ranked by importance</span>
          </div>
          <div className="cards">
            {top.map((it, i) => (
              <Card key={it.id} it={it} now={now} eager={i < 3} mode={view.mode} />
            ))}
          </div>
        </section>
      )}

      <Banner />
      {blocks}
      <EditionEnd />
    </>
  );
}

// Section: the chosen category's articles, newest first, with its sources as filters.
export async function SectionView({ view, sources, now }: { view: View; sources: SourceCount[]; now: number }) {
  const body = await getDigest({ days: view.days, category: view.category, source: view.source, page: view.page, pageSize: PAGE_SIZE, detail: view.mode === "7" });
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
            i === 0 && body.page === 1 ? <Feature key={it.id} it={it} now={now} mode={view.mode} /> : <Card key={it.id} it={it} now={now} eager={i < 4} mode={view.mode} />,
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
  const body = await getDigest({ days: view.days, q: view.q, page: view.page, pageSize: PAGE_SIZE, detail: view.mode === "7" });
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
            <Row key={it.id} it={it} now={now} withGist mode={view.mode} />
          ))}
        </div>
      )}
      <Pager view={view} page={body.page} pages={body.total_pages} />
      {lastPage && body.items.length > 0 && <EditionEnd />}
    </>
  );
}
