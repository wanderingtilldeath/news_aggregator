import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import QuizPlayer from "@/components/quiz-player";
import Revise from "@/components/revise";
import { getQuiz } from "@/lib/api";
import { TRACKS, TRACK_IDS } from "@/lib/view";

export const metadata: Metadata = { title: "Daily quiz · Postpaper", description: "Five questions from today's stories." };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const sp = await searchParams;
  const track = typeof sp.track === "string" && TRACK_IDS.includes(sp.track) ? sp.track : TRACK_IDS[0];
  const day = typeof sp.day === "string" && /^\d{4}-\d{2}-\d{2}$/.test(sp.day) ? sp.day : undefined;
  const quiz = await getQuiz(track, day);

  return (
    <>
      <nav className="subnav" aria-label="Editions">
        {TRACK_IDS.map((t) => (
          <Link key={t} href={`/quiz?track=${t}`} aria-current={t === track ? "true" : undefined}>
            {TRACKS[t].title}
          </Link>
        ))}
      </nav>
      {quiz && (
        <p className="lead-in">
          {TRACKS[track].title}, edition of {quiz.day}. Pick an answer to see the explanation.
        </p>
      )}
      <div className="split-main">
        {quiz ? (
          <QuizPlayer key={`${quiz.track}-${quiz.day}`} questions={quiz.questions} />
        ) : (
          <div className="empty-card">
            <h2>{day ? `No quiz for ${day}` : "Today\u2019s quiz is not ready yet"}</h2>
            <p>A new five-question quiz is written with each morning&rsquo;s edition, from the day&rsquo;s most important stories.</p>
            <Link className="btn primary" href="/archive">
              Browse past editions
            </Link>
          </div>
        )}
        <Revise />
        <p>
          <Link href="/">Back to the news</Link>
        </p>
      </div>
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/quiz">) {
  return (
    <main className="page split" id="view">
      <h1>Daily quiz</h1>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
