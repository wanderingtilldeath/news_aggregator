import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import { connection } from "next/server";
import GraphSvg from "@/components/graph-svg";
import { getGraph } from "@/lib/api";
import { ego, entityHref } from "@/lib/graphlayout";
import { ago, cleanTitle, hostName } from "@/lib/view";

export async function generateMetadata({ params }: PageProps<"/entity/[name]">): Promise<Metadata> {
  const name = decodeURIComponent((await params).name);
  return { title: `${name} · Postpaper`, description: `The stories about ${name} and the names that appear with it.` };
}

const SIZE = 460;

async function Entity({ params }: { params: Promise<{ name: string }> }) {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const name = decodeURIComponent((await params).name);
  const graph = await getGraph();
  const node = graph?.entities.find((n) => n.name.toLowerCase() === name.toLowerCase());
  if (!graph || !node) notFound();

  const known = new Set(graph.entities.map((n) => n.name));
  const { centre, around } = ego(node, SIZE);
  return (
    <>
      <h1>{node.name}</h1>
      <p className="lead-in">
        In {node.count} {node.count === 1 ? "story" : "stories"} in the last two weeks.
      </p>
      <div className="entity-grid">
        <section aria-label="Stories">
          <h2 className="gr-head">Stories</h2>
          <ul className="entity-stories">
            {node.stories.map((s) => (
              <li key={s.id}>
                <Link href={`/story/${encodeURIComponent(s.id)}`}>{cleanTitle(s.title)}</Link>
                {s.why && <p>{s.why}</p>}
                <p className="up-src">
                  {hostName(s.source)} · {ago(s.time, now)}
                </p>
              </li>
            ))}
          </ul>
        </section>
        {node.related.length > 0 && (
          <section aria-label="Named alongside">
            <h2 className="gr-head">
              Named alongside <Link href="/graph">See the whole graph →</Link>
            </h2>
            <GraphSvg size={SIZE} placed={around} links={around.map((a) => ({ a: centre, b: a, count: a.count }))} centre={centre} label={`${node.name} and the names that appear in the same stories`} />
            <p className="chips">
              {node.related.map((r) =>
                known.has(r.name) ? (
                  <Link key={r.name} className="chip" href={entityHref(r.name)}>
                    {r.name} ({r.count})
                  </Link>
                ) : (
                  <span key={r.name} className="chip">
                    {r.name} ({r.count})
                  </span>
                ),
              )}
            </p>
          </section>
        )}
      </div>
    </>
  );
}

export default function Page({ params }: PageProps<"/entity/[name]">) {
  return (
    <main className="page wide-page" id="view">
      <Suspense fallback={null}>
        <Entity params={params} />
      </Suspense>
      <p>
        <Link href="/graph">Who is in the news</Link> · <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
