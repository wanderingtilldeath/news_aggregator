import Link from "next/link";
import { connection } from "next/server";
import { getSources } from "@/lib/api";
import { DAYS, DAY_LABELS, TRACKS, TRACK_IDS, catName, categoriesPresent, hrefFor, parseView } from "@/lib/view";
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

  return (
    <>
      <div className="controls">
        <div className="wrap">
          <nav className="pills" aria-label="Topics">
            <Link href={hrefFor(view, { category: "" })} aria-current={!view.category && !view.q ? "page" : undefined}>
              {view.track ? TRACKS[view.track].title : "All stories"}
            </Link>
            {cats.map((c) => (
              <Link key={c} href={hrefFor(view, { category: c })} aria-current={view.category === c && !view.q ? "page" : undefined}>
                {catName(c)}
              </Link>
            ))}
          </nav>
          <div className="options">
            <nav className="pills" aria-label="Editions">
              <span className="label">Edition</span>
              <Link href={hrefFor(view, { track: "", category: "" })} aria-current={!view.track ? "true" : undefined}>
                All
              </Link>
              {TRACK_IDS.map((t) => (
                <Link key={t} href={hrefFor(view, { track: t, category: "" })} aria-current={view.track === t ? "true" : undefined}>
                  {t === "upsc" ? "UPSC" : TRACKS[t].title}
                </Link>
              ))}
            </nav>
            <nav className="pills" aria-label="Period">
              <span className="label">Period</span>
              {DAYS.map((d) => (
                <Link key={d} href={hrefFor(view, { days: d, category: view.category, q: view.q })} aria-current={d === view.days ? "true" : undefined}>
                  {DAY_LABELS[d]}
                </Link>
              ))}
            </nav>
            <nav className="pills" aria-label="Reading length">
              <span className="label">Read</span>
              {[
                ["", "Standard"],
                ["1", "1 min"],
                ["7", "7 min"],
              ].map(([m, label]) => (
                <Link key={m} href={hrefFor(view, { mode: m, category: view.category, source: view.source, q: view.q })} aria-current={view.mode === m ? "true" : undefined}>
                  {label}
                </Link>
              ))}
            </nav>
          </div>
        </div>
      </div>

      <main id="view" className="content">
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
