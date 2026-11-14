import { connection } from "next/server";
import { getSources } from "@/lib/api";
import { TRACKS, parseView } from "@/lib/view";
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

  const total = sources.reduce((n, s) => n + s.count, 0);
  const hosts = new Set(sources.map((s) => s.source)).size;

  return (
    <main id="view" className="content">
      {/* The strap line, between two rules, the way a front page counts what is inside. */}
      <p className="strap num">
        {total.toLocaleString("en")} stories · {hosts} sources · ranked by importance
      </p>
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
