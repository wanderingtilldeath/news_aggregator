"use client";

import { useState } from "react";
import { arrow, closeness, distanceKm, type Country } from "@/lib/games";
import { SITE_URL } from "@/lib/site";
import { MAX_GUESSES, dayNumber } from "@/lib/wordle";
import { copyText, useSaved } from "./use-saved";

type Lite = Pick<Country, "name" | "lat" | "lon">;

// The outline of a country; each guess says how far away it was and which way to go.
export default function CountryGuess({ day, target, list }: { day: string; target: Country; list: Lite[] }) {
  const [guesses, setGuesses] = useSaved("pp-countries", `${day}~${target.name}`);
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  const by = (n: string) => list.find((c) => c.name.toLowerCase() === n.trim().toLowerCase());
  const won = guesses.includes(target.name);
  const over = won || guesses.length >= MAX_GUESSES;
  const rows = guesses.map((g) => {
    const c = by(g)!;
    const km = distanceKm(c as Country, target);
    return { name: c.name, km, arrow: arrow(c as Country, target), near: closeness(km) };
  });

  function guess(e: React.FormEvent) {
    e.preventDefault();
    const c = by(typed);
    setTyped("");
    if (!c) return setMessage("Pick a country from the suggestions");
    if (guesses.includes(c.name)) return setMessage("Already guessed");
    setMessage("");
    setGuesses([...guesses, c.name]);
  }

  async function share() {
    const lines = rows.map((r) => (r.km === 0 ? "🎯 🟩🟩🟩🟩🟩" : `${r.arrow} ${"🟩".repeat(Math.round(r.near / 20))}${"⬛".repeat(5 - Math.round(r.near / 20))}`));
    setMessage(await copyText(`Postpaper Country guess ${dayNumber(day)} ${won ? guesses.length : "X"}/${MAX_GUESSES}\n${lines.join("\n")}\n${SITE_URL}/games/countries`));
  }

  return (
    <div className="cg">
      <svg className="cg-map" viewBox="0 0 100 100" role="img" aria-label="Outline of the country to guess">
        <path d={target.d} transform={`translate(${(100 - target.w) / 2} ${(100 - target.h) / 2})`} fillRule="evenodd" />
        <title>{over ? target.name : "Outline of the country to guess"}</title>
      </svg>

      <ol className="cg-rows">
        {rows.map((r) => (
          <li key={r.name} className={r.km === 0 ? "hit" : ""}>
            <b>{r.name}</b>
            <span className="cg-km">{r.km === 0 ? "Correct" : `${r.km.toLocaleString("en")} km ${r.arrow}`}</span>
            <span className="cg-bar" aria-label={`${r.near}% close`}>
              <i style={{ width: `${r.km === 0 ? 100 : r.near}%` }} />
            </span>
          </li>
        ))}
      </ol>

      {over ? (
        <div className="wd-end">
          <p className="wd-msg">{won ? "You got it." : `It was ${target.name}.`}</p>
          <button type="button" className="btn primary" onClick={share}>
            Share your result
          </button>
          <p className="fine-print">{message || "A new country every day."}</p>
        </div>
      ) : (
        <>
          <form className="lad-form" onSubmit={guess}>
            <input value={typed} onChange={(e) => setTyped(e.target.value)} list="countries" placeholder="Type a country" aria-label="Country" autoComplete="off" spellCheck={false} />
            <datalist id="countries">
              {list.map((c) => (
                <option key={c.name} value={c.name} />
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
