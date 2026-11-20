import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = { title: "Games · Postpaper", description: "A new puzzle every day: Wordle, a headline with a word missing, Spelling Bee, Word ladder, a country from its outline, and who is in the news." };

const GAMES = [
  { href: "/games/wordle", name: "Daily Wordle", text: "Guess the five-letter word in six tries." },
  { href: "/games/headline", name: "Headline blank", text: "One word of a real headline is missing. Find it." },
  { href: "/games/bee", name: "Spelling Bee", text: "Make words from seven letters. Find the one that uses them all." },
  { href: "/games/ladder", name: "Word ladder", text: "Get from one word to another, changing one letter at a time." },
  { href: "/games/countries", name: "Country guess", text: "Name the country from its outline, with distance and direction hints." },
  { href: "/games/who", name: "Who is it?", text: "Guess the name in the news from headlines with the name taken out." },
];

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Games</h1>
      <p className="lead-in">A new puzzle every day, the same for everyone. Your scores and streaks stay in your browser.</p>
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
