import { connection } from "next/server";
import { getSources } from "@/lib/api";
import { parseView } from "@/lib/view";
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

  return (
    <main id="view" className="content">
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
