import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import { getEvents, type NewsEvent } from "@/lib/api";
import { cleanTitle, hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Upcoming · Postpaper",
  description: "Dates the news says are coming: meetings, deadlines, summits and decisions, each linked to the story it came from.",
};

const dayLabel = (day: string) => new Date(`${day}T12:00:00Z`).toLocaleDateString("en-IN", { weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });

async function Calendar() {
  await connection();
  const events = await getEvents();
  if (!events.length) return <p className="empty">No upcoming dates found in the news yet. They are collected a little each day from the stories.</p>;

  const byDay = new Map<string, NewsEvent[]>();
  for (const e of events) byDay.set(e.day, [...(byDay.get(e.day) ?? []), e]);

  return (
    <div className="upcoming">
      {[...byDay.entries()].map(([day, list]) => (
        <section key={day} className="up-day">
          <h2>{dayLabel(day)}</h2>
          <ul>
            {list.map((e) => (
              <li key={`${e.story}-${e.text}`}>
                <p>{e.text}</p>
                <p className="up-src">
                  From <Link href={`/story/${encodeURIComponent(e.story)}`}>{cleanTitle(e.title)}</Link> · {hostName(e.source)}
                </p>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </div>
  );
}

export default function Page() {
  return (
    <main className="page">
      <h1>Upcoming</h1>
      <p className="lead-in">
        Dates the news says are coming, found in the stories of the last day or two. <a href="/upcoming.ics">Add them to your calendar</a> (a subscription that updates itself).
      </p>
      <p className="fine-print">Dates are picked out of the text by rules, not checked by a person: open the story and the publisher before you plan around one.</p>
      <Suspense fallback={null}>
        <Calendar />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
