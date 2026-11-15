import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import PrintButton from "@/components/print-button";
import { rankByTrack } from "@/components/ranked";
import { getDigest, getIntro, getProStatus, getQuiz, type Item } from "@/lib/api";
import { TRACKS, TRACK_IDS, catName, cleanTitle, hostName } from "@/lib/view";

export const metadata: Metadata = { title: "Printable edition · Postpaper Pro", robots: { index: false, follow: false } };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";

function Story({ it }: { it: Item }) {
  const bullets = (it.summary ?? []).filter(Boolean);
  return (
    <li className="pro-story">
      <h3>{cleanTitle(it.title || it.url)}</h3>
      {it.why && (
        <p className="why">
          <b>Why it matters:</b> {it.why}
        </p>
      )}
      {bullets.length > 0 && (
        <ul>
          {bullets.map((b, i) => (
            <li key={i}>{b}</li>
          ))}
        </ul>
      )}
      {it.long && <p>{it.long}</p>}
      <p className="meta">
        {hostName(it.source)} · {catName(it.category || "other")}
        {it.tags?.map((t) => ` · #${t}`)}
      </p>
    </li>
  );
}

// One edition for printing: per track the briefing, the ranked stories in full and the quiz with its
// answers. ?day= picks an edition day and ?scope=week the last seven days (stories only).
async function Edition({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  await connection();
  const sp = await searchParams;
  const token = first(sp.token);
  const status = token ? await getProStatus(token) : null;

  if (status === "error") return <p className="empty">We could not check your link just now. Please try again in a minute.</p>;
  if (!status) {
    return (
      <p className="empty">
        This link is not valid. Use the link in your latest Pro email, or <Link href="/pro">see Postpaper Pro</Link>.
      </p>
    );
  }
  if (!status.active) {
    return (
      <p className="empty">
        Your Pro membership ended on {status.until}. <Link href="/pro">Renew it</Link> to open the printable editions again.
      </p>
    );
  }

  const week = first(sp.scope) === "week";
  const day = /^\d{4}-\d{2}-\d{2}$/.test(first(sp.day)) ? first(sp.day) : undefined;
  const cats = TRACK_IDS.flatMap((t) => TRACKS[t].categories);
  const pages = await Promise.all(
    cats.map((c) => getDigest({ days: week ? "7" : day ? "1" : "2", day: week ? undefined : day, category: c, pageSize: 60, detail: true }).catch(() => null)),
  );
  const groups = rankByTrack(
    pages.flatMap((p) => p?.items ?? []),
    week ? 15 : 8,
  );
  if (!groups.length) return <p className="empty">Nothing in this edition yet.</p>;

  const extras = week
    ? {}
    : Object.fromEntries(
        await Promise.all(
          groups.map(async (g) => [g.id, { intro: await getIntro(g.id, day).catch(() => null), quiz: await getQuiz(g.id, day).catch(() => null) }] as const),
        ),
      );

  return (
    <>
      <PrintButton />
      {groups.map((g) => {
        const { intro, quiz } = extras[g.id] ?? {};
        return (
          <section key={g.id} className="pro-section">
            <h2>{g.title}</h2>
            {intro && (
              <p className="pro-intro">
                <b>Today in 60 seconds.</b> {intro.text}
              </p>
            )}
            <ol className="pro-stories">
              {g.items.map((it) => (
                <Story key={it.id} it={it} />
              ))}
            </ol>
            {quiz && quiz.questions.length > 0 && (
              <div className="pro-quiz">
                <h3>Quiz, with answers</h3>
                <ol>
                  {quiz.questions.map((q, i) => (
                    <li key={i}>
                      {q.question}
                      <ul className="plain">
                        {q.options.map((o, j) => (
                          <li key={j}>
                            {String.fromCharCode(65 + j)}. {o}
                            {j === q.answer ? " ✓" : ""}
                          </li>
                        ))}
                      </ul>
                      <small>{q.explanation}</small>
                    </li>
                  ))}
                </ol>
              </div>
            )}
          </section>
        );
      })}
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/pro/edition">) {
  return (
    <main className="page">
      <h1>Postpaper Pro · printable edition</h1>
      <p className="lead-in">Use your browser&rsquo;s print dialog to save this page as a PDF. Keep your link private.</p>
      <Suspense fallback={null}>
        <Edition searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
