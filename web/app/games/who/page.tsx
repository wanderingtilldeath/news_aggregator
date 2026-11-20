import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import WhoIsIt from "@/components/who-is-it";
import { getGraph } from "@/lib/api";
import { maskName, whoOfDay } from "@/lib/games";
import { cleanTitle } from "@/lib/view";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Who is it? · Postpaper", description: "Guess the name in the news from headlines with the name taken out." };

async function Body() {
  await connection();
  const day = dayKey(new Date());
  const graph = await getGraph();
  const who = graph ? whoOfDay(graph.entities, day) : null;
  if (!graph || !who) return <p className="empty">There is no puzzle right now. Please try again in a while.</p>;
  // The name is taken out of each headline here, on the server, so the clues never carry it.
  const clues = [...who.stories].sort((a, b) => a.id.localeCompare(b.id)).slice(0, 6).map((s) => ({ id: s.id, title: maskName(cleanTitle(s.title), who.name), source: s.source }));
  return <WhoIsIt day={day} answer={who.name} clues={clues} names={graph.entities.map((e) => e.name).sort()} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Who is it?</h1>
      <p className="lead-in">A person, place or organisation from this fortnight&rsquo;s news. The headlines have the name taken out. Each wrong guess shows another one.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
