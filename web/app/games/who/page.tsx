import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import WhoIsIt from "@/components/who-is-it";
import { getGraph } from "@/lib/api";
import { whoClues, whoOfDay } from "@/lib/games";
import { cleanTitle } from "@/lib/view";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Who is it? · Postpaper", description: "Guess the name in the news from headlines with the name taken out." };

async function Body() {
  await connection();
  const day = dayKey(new Date());
  const graph = await getGraph();
  // Headlines are cleaned (the " | Site" tail removed) before any name is blanked or clue chosen.
  const entities = graph ? graph.entities.map((e) => ({ ...e, stories: e.stories.map((s) => ({ ...s, title: cleanTitle(s.title) })) })) : [];
  const who = whoOfDay(entities, day);
  if (!graph || !who) return <p className="empty">There is no puzzle right now. Please try again in a while.</p>;
  // The name is taken out of each headline here, on the server, so a clue never carries it as shown.
  const clues = whoClues(who.stories, who.name);
  const publishers = new Set(who.stories.map((s) => s.source.replace(/^www\./, ""))).size;

  return <WhoIsIt day={day} answer={who.name} clues={clues} names={graph.entities.map((e) => e.name).sort()} stories={who.count} publishers={publishers} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Who is it?</h1>
      <p className="lead-in">
        Who or what is in the news? Every headline below is a real one from the last two weeks with the name blanked out (<b>▮▮▮</b>). You start with three headlines; each wrong guess adds another, and two hints follow. You have six
        guesses, and a surname or a one-word short form counts.
      </p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
