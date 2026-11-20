"use client";

import { type ReactNode, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { SITE_URL } from "@/lib/site";
import { pickFresh } from "./use-saved";
import { ANSWERS, MAX_GUESSES, WORD_LENGTH, dayNumber, nextStats, readStats, score, shareText, wordOfDay, dayKey, type Mark } from "@/lib/wordle";

const ROWS = ["qwertyuiop", "asdfghjkl", "zxcvbnm"];
const RANK: Record<Mark, number> = { absent: 1, present: 2, correct: 3 };

const noop = () => () => {};

type Props = {
  slug: string; // the route under /games, and the browser storage key
  name: string; // shown in the shared result
  answer?: string; // the word, when the page picks it (Headline blank); Wordle takes it from the date
  day?: string; // the puzzle's day when the page picks it
  intro?: ReactNode; // shown above the board (the headline with its blank)
  after?: ReactNode; // shown when the game is over (the full headline and its link)
};

// Six tries at the day's word, any length. Today's guesses and the stats live in the reader's browser only.
export default function WordGame({ slug, name, answer: fixed, day, intro, after }: Props) {
  const GUESSES_KEY = `pp-${slug}`; // "id|guess,guess"
  const STATS_KEY = `pp-${slug}-stats`;
  // The date is read in the browser (empty on the server), so the page bakes in no word.
  const clientDay = useSyncExternalStore(noop, () => dayKey(new Date()), () => "");
  const today = day ?? clientDay;
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(GUESSES_KEY), () => "");
  const stats = readStats(useSyncExternalStore(subscribeLocal, () => readLocal(STATS_KEY), () => ""));
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  // Practice: more rounds with other random words once the day's word is done (Wordle only). Kept in memory, so
  // it never touches the saved game, the stats or the streak, and a reload returns to today's result.
  // The guesses Wordle accepts (real words), loaded in the background; until it arrives any word is let through.
  const dict = useRef<Set<string> | null>(null);
  useEffect(() => {
    if (!fixed) import("@/lib/data/words5.json").then((m) => void (dict.current = new Set(m.default)));
  }, [fixed]);
  const [practice, setPractice] = useState<{ answer: string; guesses: string[] } | null>(null);

  const answer = practice?.answer ?? fixed ?? (today ? wordOfDay(today) : "");
  const length = answer.length || WORD_LENGTH;
  const id = fixed ? `${today}~${fixed}` : today; // a puzzle that changed under a saved game starts fresh
  const [savedId, saved = ""] = raw.split("|");
  const guesses = practice ? practice.guesses : today && savedId === id && saved ? saved.split(",") : [];
  const won = guesses[guesses.length - 1] === answer && answer !== "";
  const over = won || guesses.length >= MAX_GUESSES;

  const letters: Record<string, Mark> = {};
  for (const g of guesses) score(g, answer).forEach((m, i) => (RANK[m] > (RANK[letters[g[i]]] ?? 0) ? (letters[g[i]] = m) : null));

  function press(key: string) {
    if (!today || over) return;
    setMessage("");
    if (key === "Backspace") return setTyped((t) => t.slice(0, -1));
    if (key === "Enter") {
      if (typed.length < length) return setMessage("Not enough letters");
      if (!fixed && dict.current && !dict.current.has(typed) && !ANSWERS.includes(typed)) return setMessage("Not in the word list");
      const next = [...guesses, typed];
      if (practice) {
        setPractice({ ...practice, guesses: next });
        setTyped("");
        return;
      }
      writeLocal(GUESSES_KEY, `${id}|${next.join(",")}`);
      setTyped("");
      if (typed === answer || next.length >= MAX_GUESSES) writeLocal(STATS_KEY, nextStats(readLocal(STATS_KEY), typed === answer, today));
      return;
    }
    if (/^[a-z]$/.test(key) && typed.length < length) setTyped((t) => t + key);
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
    // Practice words: the common five-letter words (loaded on the click), none repeated until all have been played.
    const { default: answers } = await import("@/lib/data/answers5.json");
    setPractice({ answer: pickFresh(answers.filter((w) => w !== answer && w !== wordOfDay(today)), `pp-${slug}-seen`), guesses: [] });
    setTyped("");
    setMessage("");
  }

  async function share() {
    try {
      await navigator.clipboard.writeText(`${shareText(guesses, answer, today, name)}\n${SITE_URL}/games/${slug}`);
      setMessage("Copied. Paste it to share.");
    } catch {
      setMessage("Could not copy. Select the grid and copy it instead.");
    }
  }

  return (
    <div className="wd" style={{ "--n": length } as React.CSSProperties}>
      {intro}
      {practice && <p className="fine-print">Practice word. It does not count towards your streak.</p>}
      <div className="wd-board" role="grid" aria-label={practice ? `${name} practice` : today ? `${name} number ${dayNumber(today)}` : name}>
        {Array.from({ length: MAX_GUESSES }, (_, r) => {
          const guess = guesses[r] ?? (r === guesses.length && !over ? typed : "");
          const marks = guesses[r] ? score(guesses[r], answer) : [];
          return (
            <div className="wd-row" role="row" key={r}>
              {Array.from({ length }, (_, c) => (
                <span key={c} role="gridcell" className={`wd-tile ${marks[c] ?? (guess[c] ? "filled" : "")}`} aria-label={guess[c] ? `${guess[c]}${marks[c] ? `, ${marks[c]}` : ""}` : "empty"}>
                  {guess[c]}
                </span>
              ))}
            </div>
          );
        })}
      </div>

      <p className="wd-msg" role="status">
        {message || (over ? (won ? "You got it." : `The word was ${answer.toUpperCase()}.`) : "")}
      </p>

      {over ? (
        <div className="wd-end">
          {!practice && (
            <>
              <button type="button" className="btn primary" onClick={share}>
                Share your result
              </button>
              <p className="wd-stats">
                Played {stats.played} · Won {stats.played ? Math.round((stats.won / stats.played) * 100) : 0}% · Streak {stats.streak}
              </p>
            </>
          )}
          {after}
          {!fixed && (
            <button type="button" className={practice ? "btn primary" : "more"} onClick={another}>
              Play another word
            </button>
          )}
          {practice && (
            <button type="button" className="more" onClick={() => setPractice(null)}>
              Back to today&rsquo;s word
            </button>
          )}
          {!practice && <p className="fine-print">{fixed ? "A new puzzle every day." : "A new daily word every day. Practice words do not count towards your streak."}</p>}
        </div>
      ) : (
        <div className="wd-keys" aria-label="Keyboard">
          {ROWS.map((row, i) => (
            <div className="wd-keyrow" key={row}>
              {i === 2 && (
                <button type="button" className="wd-key wide" onMouseDown={(e) => e.preventDefault()} onClick={() => press("Enter")}>
                  Enter
                </button>
              )}
              {[...row].map((k) => (
                <button type="button" key={k} className={`wd-key ${letters[k] ?? ""}`} onMouseDown={(e) => e.preventDefault()} onClick={() => press(k)}>
                  {k}
                </button>
              ))}
              {i === 2 && (
                <button type="button" className="wd-key wide" aria-label="Delete" onMouseDown={(e) => e.preventDefault()} onClick={() => press("Backspace")}>
                  ⌫
                </button>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
