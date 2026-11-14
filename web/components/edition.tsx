import Link from "next/link";
import { connection } from "next/server";
import { getSources } from "@/lib/api";
import { TRACKS, catName, categoriesPresent, hrefFor, parseView } from "@/lib/view";
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
  const total = sources.reduce((n, s) => n + s.count, 0);
  const hosts = new Set(sources.map((s) => s.source)).size;
  const date = new Date(now).toLocaleDateString("en-GB", { weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Kolkata" });

  return (
    <main id="view" className="content">
      {/* The dateline: today's date, the sections as plain links like a newspaper's, and the size of the edition. */}
      <div className="dateline">
        <span className="date">{date}</span>
        <nav className="sections" aria-label="Sections">
          <Link href={hrefFor(view, { category: "" })} aria-current={!view.category && !view.q ? "page" : undefined}>
            Top
          </Link>
          {cats.map((c) => (
            <Link key={c} href={hrefFor(view, { category: c })} aria-current={view.category === c && !view.q ? "page" : undefined}>
              {catName(c)}
            </Link>
          ))}
        </nav>
        <span className="tally num">
          {total.toLocaleString("en")} stories · {hosts} sources
        </span>
      </div>
      {!view.category && !view.q && (view.track ? <h1 className="front-title">{TRACKS[view.track].title}</h1> : <h1 className="sr-only">Postpaper, today&rsquo;s edition</h1>)}
      {view.q ? (
        <SearchView view={view} now={now} />
      ) : view.category ? (
        <SectionView view={view} sources={sources} now={now} />
      ) : (
        <HomeView view={view} sources={sources} now={now} />
      )}
    </main>
  );
}
