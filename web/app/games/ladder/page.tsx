import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import WordLadder from "@/components/word-ladder";
import ladders from "@/lib/data/ladders.json";
import valid from "@/lib/data/words4.json";
import { ladderOfDay } from "@/lib/games";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Word ladder · Postpaper", description: "Change one letter at a time to get from one word to another." };

async function Body() {
  await connection();
  const day = dayKey(new Date());
  return <WordLadder day={day} puzzle={ladderOfDay(ladders, day)} valid={valid} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Word ladder</h1>
      <p className="lead-in">Get from the first word to the last by changing one letter at a time. Every step must be a real word. Fewer steps is better.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
