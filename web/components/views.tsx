import Link from "next/link";
import { getDigest, type Item, type SourceCount } from "@/lib/api";
import SectionFront, { SECTION_FRONT } from "./section-front";
import { PAGE_SIZE, TRACKS, catName, cleanTitle, categoriesPresent, hostName, hrefFor, pageList, score, type View } from "@/lib/view";
import { Byline, Card, MainStory, RankItem, Row, TextStory, ThumbRow, fullText, storyHref } from "./article";
import { fill, heightLatest, heightMain, heightRank, heightRow, heightLeft, total } from "@/lib/balance";
import Banner from "./hero";
import Briefing from "./briefing";
import Discover from "./discover";
import FundingBlock from "./funding-block";
import Streak from "./streak";

const Empty = ({ text }: { text: string }) => <div className="empty">{text}</div>;

// The marker that makes this a finished edition rather than an endless feed.
export const EditionEnd = () => (
  <>
    <p className="done" role="status">
      That is today&rsquo;s edition. You are caught up.
    </p>
    <Streak />
  </>
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

// Home: a newspaper front page. Three columns that end on the same line (text stories; the main
// story with three more beside a thumbnail; numbered top stories and the latest), a dark band of
// four, then more top stories and a row per section. A story is shown once.
export async function HomeView({ view, sources, now }: { view: View; sources: SourceCount[]; now: number }) {
  const present = categoriesPresent(sources.map((s) => s.category));
  // An edition (track) shows only its own categories.
  const cats = view.track ? present.filter((c) => TRACKS[view.track].categories.includes(c)) : present;
  const detail = view.mode === "7";
  const [latest, ...sections] = await Promise.all([
    view.track ? Promise.resolve(null) : getDigest({ days: view.days, pageSize: 40, detail }),
    ...cats.map((c) => getDigest({ days: view.days, category: c, pageSize: 24, detail }).catch(() => null)),
  ]);

  // The pool the front page is built from: the newest stories overall, or, inside an edition,
  // everything its categories returned.
  const pool: Item[] = latest ? latest.items : sections.flatMap((s) => s?.items ?? []).sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
  if (!pool.length) return <Empty text="No articles yet for this period." />;

  const ranked = [...pool].sort((a, b) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time));
  const main = ranked.find((i) => i.image) ?? ranked[0];
  const rest = ranked.filter((i) => i !== main);
  // The three columns should end on the same line, so the middle one (the main story and three
  // thumbnail rows) sets the height and the left and right columns take as many stories as fit it.
  const gist = (it: Item) => (view.mode === "7" && it.long ? it.long : fullText(it));
  const rows = rest.slice(0, 3);
  const target = heightMain(main, gist(main)) + total(rows, (it) => heightRow(it, gist(it)));
  let leftover = rest.slice(3);
  const left = fill(leftover, (it) => heightLeft(it, gist(it)), target);
  leftover = leftover.slice(left.length);
  const topHead = 50;
  const trending = fill(leftover, heightRank, target - topHead, 8);
  leftover = leftover.slice(trending.length);
  // What is left of the right column's height goes to the newest stories not shown yet.
  const room = target - topHead - total(trending, heightRank) - 70;
  const newest = room > 110 ? fill([...leftover].sort((a, b) => Date.parse(b.time) - Date.parse(a.time)), heightLatest, room, 5) : [];
  leftover = leftover.filter((i) => !newest.includes(i));
  const band = leftover.slice(0, 4);
  const more = leftover.slice(4, 12);
  const used = new Set([main, ...left, ...rows, ...trending, ...newest, ...band, ...more].map((i) => i.id));

  const blocks = cats.flatMap((c, i) => {
    // The front page and the bands above have already taken the newest stories of every section, so each
    // section is fetched with room to spare (24) to still have a row of four left for its own block. A
    // section with fewer than two left shows no block: one lone card in a wide row looks broken, and its
    // stories are all under "See all" in the bar.
    const items = (sections[i]?.items ?? []).filter((it) => !used.has(it.id)).slice(0, 4);
    if (items.length < 2) return [];
    items.forEach((it) => used.add(it.id));
    return [
      <section key={c}>
        <div className="sec-head">
          <h2>{catName(c)}</h2>
          <Link className="more" href={hrefFor(view, { category: c })}>
            See all {catName(c)} <span aria-hidden="true">→</span>
          </Link>
        </div>
        <div className="cards four">
          {items.map((it) => (
            <Card key={it.id} it={it} now={now} mode={view.mode} />
          ))}
        </div>
      </section>,
    ];
  });

  return (
    <>
      <section className="front" aria-label="Front page">
        <div className="col-left">
          {left.map((it) => (
            <TextStory key={it.id} it={it} now={now} mode={view.mode} />
          ))}
        </div>

        <div className="col-main">
          <MainStory it={main} now={now} mode={view.mode} />
          {rows.map((it) => (
            <ThumbRow key={it.id} it={it} now={now} mode={view.mode} />
          ))}
        </div>

        <aside className="col-right" aria-label="Lists">
          {trending.length > 0 && (
            <>
              <h2 className="list-head">Top stories</h2>
              <ol className="rank-list">
                {trending.map((it, i) => (
                  <RankItem key={it.id} it={it} n={i + 1} now={now} />
                ))}
              </ol>
            </>
          )}
          {newest.length > 0 && (
            <>
              <h2 className="list-head">Latest</h2>
              <ul className="latest-list">
                {newest.map((it) => (
                  <li key={it.id}>
                    <Link href={storyHref(it)}>{cleanTitle(it.title || it.url)}</Link>
                    <Byline it={it} now={now} />
                  </li>
                ))}
              </ul>
            </>
          )}
        </aside>
      </section>

      {/* Under the front page, not above it: the lead story stays at the top, and this summary reads
          as the wrap-up of the headlines just above. */}
      <Briefing track={view.track} />

      {band.length > 0 && (
        <section className="band-dark" aria-label="Also today">
          {band.map((it) => (
            <TextStory key={it.id} it={it} now={now} mode={view.mode} />
          ))}
        </section>
      )}

      <Discover now={now} />

      <FundingBlock now={now} />

      {more.length > 0 && (
        <section>
          <div className="sec-head">
            <h2>More top stories</h2>
            <nav className="seg" aria-label="Reading length">
              <Link href={hrefFor(view, { mode: "" })} aria-current={view.mode !== "7" ? "true" : undefined}>
                Brief
              </Link>
              <Link href={hrefFor(view, { mode: "7" })} aria-current={view.mode === "7" ? "true" : undefined}>
                Detailed
              </Link>
            </nav>
          </div>
          <div className="cards four">
            {more.map((it, i) => (
              <Card key={it.id} it={it} now={now} eager={i < 4} mode={view.mode} />
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
  // The first page opens with a section front (the lead stories laid out like a newspaper's); the rest, and every
  // later page, are cards. A section too small for a front goes straight to cards.
  const front = body.page === 1 && body.items.length >= 6 ? body.items.slice(0, SECTION_FRONT) : [];
  const rest = body.items.slice(front.length);

  return (
    <>
      <nav className="crumbs" aria-label="Breadcrumb">
        <Link href={hrefFor(view, { category: "" })}>{view.track ? TRACKS[view.track].title : "Home"}</Link>
        <span aria-hidden="true">/</span>
        <span>{catName(view.category)}</span>
      </nav>
      <div className="sectionhead">
        <h1>{catName(view.category)}</h1>
        <span className="count">{body.total.toLocaleString("en")} articles</span>
      </div>
      {mine.length > 1 && (
        <details className="filter">
          <summary>{view.source ? `Source: ${hostName(view.source)}` : "Filter by source"}</summary>
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
        </details>
      )}
      {!body.items.length ? (
        <Empty text="No articles in this section for this period." />
      ) : (
        <>
          {front.length > 0 && <SectionFront items={front} label={catName(view.category)} now={now} mode={view.mode} />}
          {rest.length > 0 && (
            <>
              {front.length > 0 && (
                <div className="sec-head">
                  <h2>More in {catName(view.category)}</h2>
                </div>
              )}
              <div className="cards">
                {rest.map((it, i) => (
                  <Card key={it.id} it={it} now={now} eager={i < 3} mode={view.mode} />
                ))}
              </div>
            </>
          )}
        </>
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
