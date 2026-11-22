import type { Metadata } from "next";
import Link from "next/link";
import { GAMES } from "@/lib/game-list";

export const metadata: Metadata = { title: "Games · Postpaper", description: "A new puzzle every day: Wordle, a headline with a word missing, Spelling Bee, Word ladder, a country from its outline, and who is in the news." };

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Games</h1>
      <p className="lead-in">A new puzzle every day, the same for everyone. Your game scores stay in your browser.</p>
      <ul className="game-grid">
        {GAMES.map((g) => (
          <li key={g.href}>
            <Link href={g.href}>
              <h2>{g.name}</h2>
              <p>{g.text}</p>
              <span className="game-play">
                Play <span aria-hidden="true">→</span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
