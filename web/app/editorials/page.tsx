import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import { Row } from "@/components/article";
import { getDigest, type Item } from "@/lib/api";
import { hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Editorials · Postpaper",
  description: "The editorials of The Hindu and The Indian Express, side by side, each with a summary, why it matters and its exam relevance.",
};

// The papers we read editorials from, in the order shown; any other host that gets the "editorial"
// category in sources.json follows, under its own name.
const PAPERS: Record<string, string> = { "thehindu.com": "The Hindu", "indianexpress.com": "The Indian Express" };

// One column per paper, newest first, from the same cached digest call the section pages use
// (category "editorial"), so it adds no new API shape. Each editorial links to its story page,
// which carries the long summary, the exam lens and a button to the paper's own page.
async function Body() {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const body = await getDigest({ days: "7", category: "editorial", pageSize: 60 }).catch(() => null);
  if (!body) return <p className="empty">The editorials are not available right now. Please try again in a minute.</p>;
  if (!body.items.length) return <p className="empty">No editorials yet. They are collected a few times a day.</p>;

  const byPaper = new Map<string, Item[]>();
  for (const it of [...body.items].sort((a, b) => Date.parse(b.time) - Date.parse(a.time))) {
    const host = hostName(it.source);
    byPaper.set(host, [...(byPaper.get(host) ?? []), it]);
  }
  const order = (h: string) => (h in PAPERS ? Object.keys(PAPERS).indexOf(h) : Object.keys(PAPERS).length);
  const hosts = [...byPaper.keys()].sort((a, b) => order(a) - order(b) || a.localeCompare(b));

  return (
    <div className="ed-cols">
      {hosts.map((host) => (
        <section key={host} aria-label={PAPERS[host] ?? host}>
          <h2>{PAPERS[host] ?? host}</h2>
          <div className="results">
            {byPaper.get(host)!.map((it) => (
              <Row key={it.id} it={it} now={now} withGist />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

export default function Page() {
  return (
    <main className="page ed-page" id="view">
      <h1>Editorials</h1>
      <p className="lead-in">What the newspapers say, in one place: the editorials of the last week from The Hindu and The Indian Express, each with a short summary and why it matters.</p>
      <p className="fine-print">
        Summaries are written by an AI model from the editorial and may miss nuance; they are not the newspaper&rsquo;s words. Every item links to the paper&rsquo;s own page. Opinion belongs to the publisher.
      </p>
      <Suspense fallback={null}>
        <Body />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
