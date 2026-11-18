import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import { getMonths } from "@/lib/api";
import { monthLabel } from "@/lib/view";

export const metadata: Metadata = {
  title: "Monthly compilations · Postpaper",
  description: "The month's most important stories, grouped by syllabus area, ready to print as a PDF.",
};

async function List() {
  await connection();
  const months = await getMonths();
  if (!months.length) return <p className="empty">The first compilation appears after the next edition. It grows a little every day.</p>;

  return (
    <ul className="plain month-list">
      {months.map((m) => (
        <li key={m}>
          <Link href={`/month/${m}`}>{monthLabel(m)}</Link>
        </li>
      ))}
    </ul>
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Monthly compilations</h1>
      <p className="lead-in">
        Each month&rsquo;s most important stories, grouped by syllabus area, for revision. It is built a little every day, so the current month fills up as it goes. Use your browser&rsquo;s print
        dialog to save one as a PDF.
      </p>
      <Suspense fallback={<Loading />}>
        <List />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
