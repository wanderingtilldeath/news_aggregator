import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";

export const metadata: Metadata = { title: "Archive · Postpaper", description: "Past editions of Postpaper." };

const DAYS_SHOWN = 14;

// Edition days are counted in India time, newest first.
async function Days() {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const fmt = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Kolkata", year: "numeric", month: "2-digit", day: "2-digit" });
  const label = new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Kolkata", weekday: "long", day: "numeric", month: "long", year: "numeric" });
  const days = Array.from({ length: DAYS_SHOWN }, (_, i) => new Date(now - i * 86_400_000));

  return (
    <ul className="daylist">
      {days.map((d) => (
        <li key={fmt.format(d)}>
          <Link href={`/archive/${fmt.format(d)}`}>{label.format(d)}</Link>
        </li>
      ))}
    </ul>
  );
}

export default function Page() {
  return (
    <main className="page">
      <h1>Archive</h1>
      <p className="lead-in">Every past edition, ranked by importance. Use search on the home page to look for a topic.</p>
      <Suspense fallback={null}>
        <Days />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
