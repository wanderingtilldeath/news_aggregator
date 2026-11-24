"use client";

import Link from "next/link";
import { useState } from "react";
import { guessLabel, isRight, type Clue } from "@/lib/games";
import { entityHref } from "@/lib/graphlayout";
import { SITE_URL } from "@/lib/site";
import { MAX_GUESSES, dayNumber } from "@/lib/wordle";
import { copyText, useSaved } from "./use-saved";

// Real headlines about one name with the name blanked out. Three show to begin with and each wrong guess shows one
// more; a hint about the name's reach follows the second wrong guess and one about its spelling the fourth. A guess
// can be typed freely (a suggestion list helps), and a surname or other single word of the name counts as right.
export default function WhoIsIt({ day, answer, clues, names, stories, publishers }: { day: string; answer: string; clues: Clue[]; names: string[]; stories?: number; publishers: number }) {
  const [guesses, setGuesses] = useSaved("pp-who", `${day}~${answer}`);
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  const won = guesses.some((g) => isRight(g, answer));
  const over = won || guesses.length >= MAX_GUESSES;
  const shown = over ? clues.length : Math.min(clues.length, 3 + guesses.length);
  const words = answer.split(/\s+/);

  function guess(e: React.FormEvent) {
    e.preventDefault();
    const label = guessLabel(typed, names);
    setTyped("");
    if (label.length < 2) return setMessage("Type a name first");
    if (guesses.some((g) => g.toLowerCase() === label.toLowerCase())) return setMessage("You already tried that");
    setMessage("");
    setGuesses([...guesses, label]);
  }

  async function share() {
    setMessage(await copyText(`Postpaper Who is it? ${dayNumber(day)} ${won ? guesses.length : "X"}/${MAX_GUESSES}\n${guesses.map((g) => (isRight(g, answer) ? "🟩" : "⬛")).join("")}\n${SITE_URL}/games/who`));
  }

  return (
    <div className="who">
      <ol className="who-clues">
        {clues.slice(0, shown).map((c) => (
          <li key={c.id}>
            {/* When the game is over the real headline replaces the blanked one, linked to its story. */}
            {over ? <Link href={`/story/${encodeURIComponent(c.id)}`}>{c.full}</Link> : <b>{c.title}</b>}
            <span>{c.source.replace(/^www\./, "")}</span>
          </li>
        ))}
      </ol>
      {!over && clues.length > shown && (
        <p className="fine-print">
          {clues.length - shown} more {clues.length - shown === 1 ? "headline" : "headlines"} to come with wrong guesses.
        </p>
      )}
      {!over && guesses.length >= 2 && (
        <p className="who-hint">
          Hint: in {stories ?? clues.length} {(stories ?? clues.length) === 1 ? "story" : "stories"} from {publishers} {publishers === 1 ? "publisher" : "publishers"} this fortnight.
        </p>
      )}
      {!over && guesses.length >= 4 && (
        <p className="who-hint">
          Hint: {words.length} {words.length === 1 ? "word" : "words"}, starting with {words.map((w) => w[0]).join(" ")}.
        </p>
      )}
      {guesses.length > 0 && (
        <ul className="who-guesses">
          {guesses.map((g) => (
            <li key={g} className={isRight(g, answer) ? "hit" : ""}>
              {g}
            </li>
          ))}
        </ul>
      )}

      {over ? (
        <div className="wd-end">
          <p className="wd-msg">{won ? `You got it: ${answer}.` : `It was ${answer}.`}</p>
          <Link className="more" href={entityHref(answer)}>
            More about {answer} <span aria-hidden="true">→</span>
          </Link>
          <button type="button" className="btn primary" onClick={share}>
            Share your result
          </button>
          <p className="fine-print">{message || "A new name every day."}</p>
        </div>
      ) : (
        <>
          <form className="lad-form" onSubmit={guess}>
            <input value={typed} onChange={(e) => setTyped(e.target.value)} list="who-names" placeholder="Type a name" aria-label="Name" autoComplete="off" spellCheck={false} />
            <datalist id="who-names">
              {names.map((n) => (
                <option key={n} value={n} />
              ))}
            </datalist>
            <button type="submit">Guess</button>
          </form>
          <p className="wd-msg" role="status">
            {message}
          </p>
          <p className="fine-print">
            Guess {guesses.length + 1} of {MAX_GUESSES}
          </p>
        </>
      )}
    </div>
  );
}
