import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import GraphSvg from "@/components/graph-svg";
import { getGraph } from "@/lib/api";
import { entityHref, linksOf, overview } from "@/lib/graphlayout";

export const metadata: Metadata = {
  title: "Who is in the news · Postpaper",
  description: "The people, organisations, places and laws in the news most, and who is named alongside whom.",
};

const SIZE = 760;

async function Overview() {
  await connection();
  const graph = await getGraph();
  if (!graph?.entities.length) return <p className="empty">The graph builds up as stories are read. Check back after the next edition.</p>;

  const placed = overview(graph.entities, SIZE);
  const drawn = new Set(placed.map((p) => p.name));
  return (
    <>
      <GraphSvg size={SIZE} placed={placed} links={linksOf(graph, placed)} label="Who is in the news: the busiest names in the middle, lines between names that appear in the same stories" />
      <h2 className="gr-head">All names</h2>
      <ol className="gr-list">
        {graph.entities.slice(0, 60).map((n) => (
          <li key={n.name}>
            <Link href={entityHref(n.name)}>{n.name}</Link> <span>{n.count} {n.count === 1 ? "story" : "stories"}</span>
            {!drawn.has(n.name) && <span className="gr-more"> · not drawn</span>}
          </li>
        ))}
      </ol>
    </>
  );
}

export default function Page() {
  return (
    <main className="page wide-page">
      <h1>Who is in the news</h1>
      <p className="lead-in">The people, organisations, places and laws named most in the last two weeks. The busiest are in the middle; a line joins two names that appear in the same stories. Open one to see its stories.</p>
      <Suspense fallback={null}>
        <Overview />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
