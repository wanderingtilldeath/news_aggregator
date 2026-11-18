import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Suspense } from "react";
import { RankedSections, rankByTrack } from "@/components/ranked";
import { getDigest } from "@/lib/api";

export const metadata: Metadata = { title: "Edition · Postpaper" };

async function Body({ params }: { params: Promise<{ day: string }> }) {
  const { day } = await params;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(day) || Number.isNaN(Date.parse(day))) notFound();

  const page = await getDigest({ days: "1", day, pageSize: 200 });
  const groups = rankByTrack(page.items, 15);

  return (
    <>
      <h1>Edition of {day}</h1>
      <p className="lead-in">{page.total} stories collected that day; the most important of each edition are listed first.</p>
      {groups.length ? <RankedSections groups={groups} /> : <p className="empty">No stories were collected on {day}. Pages are kept for about 60 days.</p>}
    </>
  );
}

export default function Page({ params }: PageProps<"/archive/[day]">) {
  return (
    <main className="page" id="view">
      <Suspense fallback={null}>
        <Body params={params} />
      </Suspense>
      <p>
        <Link href="/archive">All editions</Link> · <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
