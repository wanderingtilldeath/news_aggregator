"use client";

import Link from "next/link";
import { useState } from "react";
import { entityHref } from "@/lib/graphlayout";
import { SITE_URL } from "@/lib/site";
import { MAX_GUESSES, dayNumber } from "@/lib/wordle";
import { copyText, useSaved } from "./use-saved";

type Clue = { id: string; title: string; source: string };

// Headlines with a name taken out; each wrong guess shows one more.
export default function WhoIsIt({ day, answer, clues, names }: { day: string; answer: string; clues: Clue[]; names: string[] }) {
  const [guesses, setGuesses] = useSaved("pp-who", `${day}~${answer}`);
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  const won = guesses.includes(answer);
  const over = won || guesses.length >= MAX_GUESSES;
  const shown = over ? clues.length : Math.min(clues.length, 2 + guesses.length);
  const words = answer.split(/\s+/);

  function guess(e: React.FormEvent) {
    e.preventDefault();
    const name = names.find((n) => n.toLowerCase() === typed.trim().toLowerCase());
    setTyped("");
    if (!name) return setMessage("Pick a name from the suggestions");
    if (guesses.includes(name)) return setMessage("Already guessed");
    setMessage("");
    setGuesses([...guesses, name]);
  }

  async function share() {
    setMessage(await copyText(`Postpaper Who is it? ${dayNumber(day)} ${won ? guesses.length : "X"}/${MAX_GUESSES}\n${guesses.map((g) => (g === answer ? "🟩" : "⬛")).join("")}\n${SITE_URL}/games/who`));
  }

  return (
    <div className="who">
      <ol className="who-clues">
        {clues.slice(0, shown).map((c) => (
          <li key={c.id}>
            <b>{c.title}</b>
            <span>{c.source.replace(/^www\./, "")}</span>
          </li>
        ))}
      </ol>
      {!over && guesses.length >= 3 && (
        <p className="who-hint">
          Hint: {words.length} {words.length === 1 ? "word" : "words"}, starting with {words.map((w) => w[0]).join(" ")}.
        </p>
      )}
      {guesses.length > 0 && (
        <ul className="who-guesses">
          {guesses.map((g) => (
            <li key={g} className={g === answer ? "hit" : ""}>
              {g}
            </li>
          ))}
        </ul>
      )}

      {over ? (
        <div className="wd-end">
          <p className="wd-msg">{won ? "You got it." : `It was ${answer}.`}</p>
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
