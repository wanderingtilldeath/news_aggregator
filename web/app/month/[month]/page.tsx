import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import { connection } from "next/server";
import PrintButton from "@/components/print-button";
import { getMonth, safeURL } from "@/lib/api";
import { TRACKS, TRACK_IDS, cleanTitle, hostName, monthLabel } from "@/lib/view";

const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;

export async function generateMetadata({ params }: PageProps<"/month/[month]">): Promise<Metadata> {
  const { month } = await params;
  return { title: `${MONTH.test(month) ? monthLabel(month) : "Compilation"} · Postpaper`, description: "The month's most important stories by syllabus area." };
}

async function Body({ params }: { params: Promise<{ month: string }> }) {
  await connection();
  const { month } = await params;
  if (!MONTH.test(month)) notFound();

  const compilations = (await Promise.all(TRACK_IDS.map(async (t) => ({ id: t, data: await getMonth(month, t) })))).filter((c) => c.data && c.data.areas.length);
  if (!compilations.length) notFound();

  return (
    <>
      <PrintButton />
      {compilations.map(({ id, data }) => (
        <section key={id} className="comp-track">
          <h2>{TRACKS[id].title}</h2>
          {data!.areas.map((area) => (
            <div key={area.name} className="comp-area">
              <h3>{area.name}</h3>
              <ol>
                {area.items.map((it) => {
                  const href = safeURL(it.url);
                  return (
                    <li key={it.id}>
                      <p className="comp-title">{href ? <a href={href} target="_blank" rel="noopener noreferrer">{cleanTitle(it.title)}</a> : cleanTitle(it.title)}</p>
                      {it.why && (
                        <p className="why">
                          <b>Why it matters:</b> {it.why}
                        </p>
                      )}
                      {it.bullets && it.bullets.length > 0 && (
                        <ul>
                          {it.bullets.map((b, i) => (
                            <li key={i}>{b}</li>
                          ))}
                        </ul>
                      )}
                      <p className="meta">
                        {hostName(it.source)} · {new Date(it.time).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "Asia/Kolkata" })}
                      </p>
                    </li>
                  );
                })}
              </ol>
            </div>
          ))}
        </section>
      ))}
      <p className="fine-print">Summaries are written by an AI model from the articles; every title links to the original. Check a fact against the source before you rely on it.</p>
    </>
  );
}

export default function Page({ params }: PageProps<"/month/[month]">) {
  return (
    <main className="page">
      <Suspense fallback={null}>
        <Heading params={params} />
        <Body params={params} />
      </Suspense>
      <p>
        <Link href="/month">All months</Link> · <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}

async function Heading({ params }: { params: Promise<{ month: string }> }) {
  const { month } = await params;
  return (
    <>
      <h1>{MONTH.test(month) ? monthLabel(month) : "Compilation"}: current affairs</h1>
      <p className="lead-in">The month&rsquo;s most important stories, by syllabus area. Use your browser&rsquo;s print dialog to save this page as a PDF.</p>
    </>
  );
}
