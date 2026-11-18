import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Flashcards from "@/components/flashcards";
import { getCards } from "@/lib/api";

export const metadata: Metadata = {
  title: "Flashcards · Postpaper",
  description: "Prelims flashcards from the facts in the most important stories, with spaced repetition kept in your browser.",
};

// Read when the page is opened, not while the site is built: a build must not depend on the API.
async function Deck() {
  await connection();
  const cards = await getCards();
  if (!cards) return <p className="empty">The flashcards are not available right now. Please try again in a minute.</p>;

  return <Flashcards cards={cards} />;
}

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Flashcards</h1>
      <p className="lead-in">
        Questions on the names, numbers and laws in the most important UPSC stories of the last two weeks. Cards you miss come back soon, and the ones you know come back later.
      </p>
      <Suspense fallback={null}>
        <Deck />
      </Suspense>
      <p className="fine-print">The cards are written by an AI model from the articles. Check an answer against the story before you rely on it for the exam.</p>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
