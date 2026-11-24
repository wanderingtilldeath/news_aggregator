import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import MapNews from "@/components/map-news";
import { getMapNews } from "@/lib/map-news-data";

export const metadata: Metadata = {
  title: "India map: places in the news · Postpaper",
  description: "The states, rivers, dams, cities and parks this week's stories are about, on a map of India. Pick a place to read its stories.",
};

async function Body() {
  const places = await getMapNews();
  if (!places) return <p className="empty">The news is not available right now. Try again in a few minutes.</p>;
  if (!places.length) return <p className="empty">No story of the last week names a place on these maps yet.</p>;

  return <MapNews places={places} />;
}

export default function Page() {
  return (
    <main className="content np-page" id="view">
      <nav className="crumbs" aria-label="Breadcrumb">
        <Link href="/maps">Maps</Link>
        <span aria-hidden="true">/</span>
        <span>In the news</span>
      </nav>
      <header className="np-head">
        <div>
          <h1>Places in the news</h1>
          <p className="np-count">Where this week&apos;s stories happened, on a map of India. Each story is matched to a place by name, so a story can be missed or matched to a namesake.</p>
        </div>
      </header>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
    </main>
  );
}
