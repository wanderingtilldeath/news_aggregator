import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import { Row } from "@/components/article";
import { getDigest } from "@/lib/api";
import { score } from "@/lib/view";

export const metadata: Metadata = {
  title: "Underreported · Postpaper",
  description: "Important stories that only one of the sources Postpaper reads carried.",
};

// Stories the model scored 4 or 5 that no other source in our list reported (see story grouping).
// It reads the same cached digest as the front page, so it adds no load.
async function Body() {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const body = await getDigest({ days: "2", pageSize: 200 }).catch(() => null);
  if (!body) return <p className="empty">The stories are not available right now. Please try again in a minute.</p>;

  const items = body.items
    .filter((it) => (it.importance ?? 0) >= 4 && !it.coverage)
    .sort((a, b) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time))
    .slice(0, 30);
  if (!items.length) return <p className="empty">Nothing stands out right now: every important story was reported by more than one source.</p>;

  return (
    <div className="results">
      {items.map((it) => (
        <Row key={it.id} it={it} now={now} withGist />
      ))}
    </div>
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Underreported</h1>
      <p className="lead-in">
        Important stories, from the last two days, that only one of the sources we read reported. A story other sources also covered is not listed here.
      </p>
      <p className="fine-print">
        This is a hint, not a verdict. We read about fifty sites, so a story can be missing here because another outlet covered it in different words, or because we do not read that outlet. Check the
        original before you rely on it.
      </p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
