import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import { getAhead, type AheadItem } from "@/lib/api";
import { catName, cleanTitle, hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Week ahead · Postpaper",
  description: "The votes, deadlines, meetings and launches the news names for the next two weeks, day by day, each linked to its story.",
};

const dayName = (date: string) => new Date(`${date}T00:00:00+05:30`).toLocaleDateString("en-IN", { weekday: "long", day: "numeric", month: "long", timeZone: "Asia/Kolkata" });

async function Calendar() {
  await connection();
  const ahead = await getAhead().catch(() => null);
  if (!ahead) return <p className="note err">The calendar could not be loaded just now. Please try again in a little while.</p>;
  if (!ahead.items.length) return <p className="empty">Nothing dated is on the calendar yet. It fills as stories name a date to watch, so check back after the next edition.</p>;

  // The API sends the items soonest first, so a new day starts whenever the date changes.
  const days: { date: string; items: AheadItem[] }[] = [];
  for (const it of ahead.items) {
    const last = days[days.length - 1];
    if (last?.date === it.date) last.items.push(it);
    else days.push({ date: it.date, items: [it] });
  }

  return (
    <>
      {days.map((d) => (
        <section key={d.date} className="ahead-day">
          <h2>{dayName(d.date)}</h2>
          <ul>
            {d.items.map((it) => (
              <li key={it.id}>
                <p className="ahead-what">{it.text}</p>
                <p className="ahead-from">
                  <Link href={`/story/${encodeURIComponent(it.id)}`}>{cleanTitle(it.title)}</Link>
                  <span>
                    {hostName(it.source)} · {catName(it.category)}
                  </span>
                </p>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Week ahead</h1>
      <p className="lead-in">What the news says to watch for in the next two weeks, by day.</p>
      <p className="fine-print">
        The dates are read from the stories by an AI model and can be wrong or move; open the story to check. Only events with a date are listed.
      </p>
      <Suspense fallback={<Loading />}>
        <Calendar />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
