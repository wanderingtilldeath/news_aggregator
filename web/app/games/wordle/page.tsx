import type { Metadata } from "next";
import Link from "next/link";
import WordGame from "@/components/word-game";

export const metadata: Metadata = { title: "Daily Wordle · Postpaper", description: "Guess the five-letter word of the day in six tries." };

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Daily Wordle</h1>
      <p className="lead-in">Guess the five-letter word in six tries. Green is right, gold is in the word but elsewhere, grey is not in it. Everyone gets the same word each day.</p>
      <WordGame slug="wordle" name="Wordle" />
      <p>
        <Link href="/games">All games</Link>
      </p>
    </main>
  );
}
