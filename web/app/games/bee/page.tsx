import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import SpellingBee from "@/components/spelling-bee";
import bees from "@/lib/data/bee-puzzles.json";
import common from "@/lib/data/common-words.json";
import { beePuzzleOfDay, beeWords } from "@/lib/games";
import { dayKey } from "@/lib/wordle";

export const metadata: Metadata = { title: "Spelling Bee · Postpaper", description: "Make words from seven letters. Every word needs the centre letter." };

async function Body() {
  await connection();
  const day = dayKey(new Date());
  const puzzle = beePuzzleOfDay(bees, day);
  return <SpellingBee day={day} letters={puzzle.letters} center={puzzle.center} words={beeWords(common, puzzle)} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Spelling Bee</h1>
      <p className="lead-in">Make words of four or more letters from the seven letters. Every word must use the centre letter, and letters can repeat. One word uses all seven.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
