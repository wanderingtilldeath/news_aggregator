import Link from "next/link";
import { connection } from "next/server";
import { getSources } from "@/lib/api";
import { DAYS, DAY_LABELS, DEFAULT_DAYS, TRACKS, TRACK_IDS, catName, categoriesPresent, hrefFor, parseView } from "@/lib/view";
import Hero from "./hero";
import { HomeView, SearchView, SectionView } from "./views";

type SearchParams = Record<string, string | string[] | undefined>;

// The whole dynamic page. It awaits searchParams, so with Cache Components it must
// sit inside <Suspense> (see app/page.tsx); the data calls below are cached.
export default async function Edition({ searchParams }: { searchParams: Promise<SearchParams> }) {
  const view = parseView(await searchParams);
  // Opt in to request time before reading the clock, so it is never baked into the prerender.
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();

  // A failed fetch throws to app/error.tsx.
  const sources = await getSources(view.days, view.q);
  const present = categoriesPresent(sources.map((s) => s.category));
  const cats = view.track ? present.filter((c) => TRACKS[view.track].categories.includes(c)) : present;
  const isHome = !view.category && !view.q && view.page === 1;
  const total = sources.reduce((n, s) => n + s.count, 0);
  const hosts = new Set(sources.map((s) => s.source)).size;

  const today = new Date(now).toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "Asia/Kolkata" });

  return (
    <>
      <div className="topbar">
        <div className="wrap">
          <span>{today}</span>
          <span className="stats">{total ? `${total.toLocaleString("en")} articles from ${hosts} sources` : ""}</span>
          <form action="/" role="search">
            <input type="search" name="q" defaultValue={view.q} placeholder="Search articles" aria-label="Search articles" />
            {view.days !== DEFAULT_DAYS && <input type="hidden" name="days" value={view.days} />}
            <button type="submit">Search</button>
          </form>
          <nav className="days" aria-label="Period">
            {DAYS.map((d) => (
              <Link key={d} href={hrefFor(view, { days: d, category: view.category, q: view.q })} aria-current={d === view.days ? "true" : undefined}>
                {DAY_LABELS[d]}
              </Link>
            ))}
          </nav>
        </div>
      </div>

      <header className="mast">
        <h1>
          <Link href={hrefFor(view)}>Postpaper</Link>
        </h1>
        <p>Articles from news, exam-prep and technology sites, gathered every morning</p>
      </header>

      <nav className="tracks" aria-label="Editions">
        <div className="wrap">
          <span>Edition</span>
          <Link href={hrefFor(view, { track: "", category: "" })} aria-current={!view.track ? "true" : undefined}>
            All
          </Link>
          {TRACK_IDS.map((t) => (
            <Link key={t} href={hrefFor(view, { track: t, category: "" })} aria-current={view.track === t ? "true" : undefined}>
              {TRACKS[t].title}
            </Link>
          ))}
          <span className="sep" />
          <span>Read</span>
          {[
            ["", "Standard"],
            ["1", "1 minute"],
            ["7", "7 minutes"],
          ].map(([m, label]) => (
            <Link key={m} href={hrefFor(view, { mode: m, category: view.category, source: view.source, q: view.q })} aria-current={view.mode === m ? "true" : undefined}>
              {label}
            </Link>
          ))}
          <span className="sep" />
          <Link href="/quiz">Quiz</Link>
          <Link href="/archive">Archive</Link>
          <Link href="/week">Weekly revision</Link>
        </div>
      </nav>

      <nav className="nav" aria-label="Sections">
        <div className="wrap">
          <Link href={hrefFor(view)} aria-current={!view.category && !view.q ? "page" : undefined}>
            Home
          </Link>
          {cats.map((c) => (
            <Link key={c} href={hrefFor(view, { category: c })} aria-current={view.category === c && !view.q ? "page" : undefined}>
              {catName(c)}
            </Link>
          ))}
        </div>
      </nav>

      <main id="view">
        {isHome && <Hero />}
        {view.q ? (
          <SearchView view={view} now={now} />
        ) : view.category ? (
          <SectionView view={view} sources={sources} now={now} />
        ) : (
          <HomeView view={view} sources={sources} now={now} />
        )}
      </main>
    </>
  );
}
