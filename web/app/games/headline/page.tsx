import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import WordGame from "@/components/word-game";
import { getDigest } from "@/lib/api";
import { headlineOfDay } from "@/lib/games";
import { cleanTitle } from "@/lib/view";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Headline blank · Postpaper", description: "One word of a real headline is missing. Guess it in six tries." };

// A word is hidden in a real headline from yesterday's stories. The same story for every reader today.
async function Body() {
  await connection();
  const key = dayKey(new Date());
  const body = await getDigest({ days: "3", pageSize: 200 }).catch(() => null);
  const items = (body?.items ?? []).filter((it) => it.is_article).map((it) => ({ id: it.id, title: cleanTitle(it.title), time: it.time }));
  const p = headlineOfDay(items, key);
  if (!p) return <p className="empty">There is no headline puzzle right now. Please try again in a while.</p>;

  return (
    <WordGame
      slug="headline"
      name="Headline blank"
      answer={p.answer}
      day={key}
      intro={
        <p className="hb-line">
          {p.before}
          <span className="hb-blank" role="img" aria-label={`${p.answer.length} missing letters`} style={{ width: `${p.answer.length * 0.72}em` }} />
          {p.after}
        </p>
      }
      after={
        <>
          <p className="hb-line">
            {p.before}
            <b>{p.answer}</b>
            {p.after}
          </p>
          <Link className="more" href={`/story/${encodeURIComponent(p.id)}`}>
            Read the story <span aria-hidden="true">→</span>
          </Link>
        </>
      }
    />
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Headline blank</h1>
      <p className="lead-in">One word of a real headline is missing. Guess it in six tries; the colours work as in Wordle.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
