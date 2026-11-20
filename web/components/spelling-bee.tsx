"use client";

import { useEffect, useState } from "react";
import { BEE_RANKS, beePoints, beeRank, beeWords, isPangram, makeBee } from "@/lib/games";
import { SITE_URL } from "@/lib/site";
import { dayNumber } from "@/lib/wordle";
import { copyText, readSeen, remember, useSaved } from "./use-saved";

// Seven letters, one in the middle. Words of four or more letters from those letters that use the middle one.
export default function SpellingBee({ day, letters: dayLetters, center: dayCenter, words: dayWords }: { day: string; letters: string; center: string; words: string[] }) {
  const [saved, setSaved] = useSaved("pp-bee", `${day}~${dayLetters}${dayCenter}`);
  // Another puzzle: picked in the browser from the puzzle list, loaded only when asked for. Kept in memory, so it
  // never touches the saved game.
  const [practice, setPractice] = useState<{ letters: string; center: string; words: string[]; found: string[] } | null>(null);
  const { letters, center, words } = practice ?? { letters: dayLetters, center: dayCenter, words: dayWords };
  const p = { letters, center };
  const found = practice ? practice.found : saved;
  const setFound = (f: string[]) => (practice ? setPractice({ ...practice, found: f }) : setSaved(f));
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  const [outer, setOuter] = useState([...dayLetters].filter((c) => c !== dayCenter));

  const max = words.reduce((n, w) => n + beePoints(w, p), 0);
  const points = found.reduce((n, w) => n + beePoints(w, p), 0);
  const rank = beeRank(points, max);
  const pangrams = words.filter((w) => isPangram(w, p));

  function submit() {
    if (typed.length < 4) return setMessage("Too short");
    if (!typed.includes(center)) return setMessage("Missing the centre letter");
    if (found.includes(typed)) return setMessage("Already found");
    if (!words.includes(typed)) return setMessage("Not in the word list");
    setFound([...found, typed]);
    setMessage(isPangram(typed, p) ? `Pangram! +${beePoints(typed, p)}` : `+${beePoints(typed, p)}`);
    setTyped("");
  }

  function press(key: string) {
    setMessage("");
    if (key === "Enter") return submit();
    if (key === "Backspace") return setTyped((t) => t.slice(0, -1));
    if (letters.includes(key) && typed.length < 19) setTyped((t) => t + key);
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if (e.ctrlKey || e.metaKey || e.altKey || (e.target as HTMLElement).closest("button, a, input")) return;
      const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;
      if (key === "Enter" || key === "Backspace" || /^[a-z]$/.test(key)) {
        e.preventDefault();
        press(key);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  async function another() {
    const { default: common } = await import("@/lib/data/common-words.json");
    const seen = readSeen("pp-bee-seen");
    seen.add(letters + center);
    // made in the browser from the common words; a puzzle already played is skipped (about 6,000 exist)
    const q = makeBee(common, Math.random, seen) ?? makeBee(common, Math.random);
    if (!q) return;
    remember("pp-bee-seen", q.letters + q.center);
    setPractice({ ...q, words: beeWords(common, q), found: [] });
    setOuter([...q.letters].filter((c) => c !== q.center));
    setTyped("");
    setMessage("");
  }

  async function share() {
    setMessage(await copyText(`Postpaper Spelling Bee ${dayNumber(day)}\n${rank}: ${points} points, ${found.length} of ${words.length} words${found.some((w) => isPangram(w, p)) ? ", found the pangram" : ""}\n${SITE_URL}/games/bee`));
  }

  const next = BEE_RANKS.find(([, f]) => f * max > points);
  return (
    <div className="bee">
      <p className="bee-rank">
        <b>{rank}</b> · {points} points · {found.length} of {words.length} words
        {next && <span> · {Math.ceil(next[1] * max) - points} to {next[0]}</span>}
      </p>
      <div className="bee-bar" role="progressbar" aria-label="Score" aria-valuemin={0} aria-valuemax={max} aria-valuenow={points}>
        <i style={{ width: `${max ? Math.min(100, (points / max) * 100) : 0}%` }} />
      </div>

      <p className="bee-typed" aria-live="polite">
        {[...typed].map((c, i) => (
          <span key={i} className={c === center ? "mid" : ""}>
            {c}
          </span>
        ))}
        <i className="caret" aria-hidden="true" />
      </p>
      <p className="wd-msg" role="status">
        {message}
      </p>

      <div className="bee-letters">
        {[...outer.slice(0, 3), center, ...outer.slice(3)].map((c) => (
          <button type="button" key={c} className={c === center ? "bee-key mid" : "bee-key"} onMouseDown={(e) => e.preventDefault()} onClick={() => press(c)}>
            {c}
          </button>
        ))}
      </div>
      <div className="bee-actions">
        <button type="button" className="more" onClick={() => press("Backspace")}>
          Delete
        </button>
        <button type="button" className="more" onClick={() => setOuter((o) => [...o].sort(() => Math.random() - 0.5))}>
          Shuffle
        </button>
        <button type="button" className="more" onClick={() => press("Enter")}>
          Enter
        </button>
      </div>

      <h2 className="bee-head">Found ({found.length})</h2>
      {found.length > 0 ? (
        <ul className="bee-found">
          {[...found].sort().map((w) => (
            <li key={w} className={isPangram(w, p) ? "pangram" : ""}>
              {w}
            </li>
          ))}
        </ul>
      ) : (
        <p className="fine-print">Nothing yet. There are {words.length} words, and {pangrams.length === 1 ? "one uses" : `${pangrams.length} use`} all seven letters.</p>
      )}
      {found.length > 0 && !practice && (
        <button type="button" className="btn primary" onClick={share}>
          Share your score
        </button>
      )}
      <div className="bee-actions">
        <button type="button" className="more" onClick={another}>
          Another puzzle
        </button>
        {practice && (
          <button type="button" className="more" onClick={() => { setPractice(null); setOuter([...dayLetters].filter((c) => c !== dayCenter)); setTyped(""); setMessage(""); }}>
            Back to today&rsquo;s puzzle
          </button>
        )}
      </div>
      {practice && <p className="fine-print">Practice puzzle. It is not saved.</p>}
    </div>
  );
}
