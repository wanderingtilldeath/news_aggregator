import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import PrintButton from "@/components/print-button";
import { RankedSections, rankByTrack } from "@/components/ranked";
import { getDigest } from "@/lib/api";
import { TRACKS, TRACK_IDS } from "@/lib/view";

export const metadata: Metadata = { title: "Weekly revision · Postpaper", description: "The week's most important stories, ranked, ready to print." };

// The week's stories of both editions, most important first. The browser prints it to PDF.
async function Body() {
  await connection();
  const cats = TRACK_IDS.flatMap((t) => TRACKS[t].categories);
  const pages = await Promise.all(cats.map((c) => getDigest({ days: "7", category: c, pageSize: 60 }).catch(() => null)));
  const groups = rankByTrack(pages.flatMap((p) => p?.items ?? []), 12);

  return groups.length ? (
    <>
      <PrintButton />
      <RankedSections groups={groups} />
    </>
  ) : (
    <p className="empty">Nothing to revise yet.</p>
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Weekly revision</h1>
      <p className="lead-in">The last seven days, ranked by importance. Use your browser&rsquo;s print dialog to save it as a PDF.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
