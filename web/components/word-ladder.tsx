"use client";

import { useMemo, useState } from "react";
import { isStep, type Ladder } from "@/lib/games";
import { SITE_URL } from "@/lib/site";
import { dayNumber } from "@/lib/wordle";
import { copyText, useSaved } from "./use-saved";

const Word = ({ word, prev, goal }: { word: string; prev?: string; goal?: boolean }) => (
  <div className={`lad-row${goal ? " goal" : ""}`}>
    {[...word].map((c, i) => (
      <span key={i} className={prev && prev[i] !== c ? "lad-tile changed" : "lad-tile"}>
        {c}
      </span>
    ))}
  </div>
);

// From one word to another, one letter at a time, every step a real word.
export default function WordLadder({ day, puzzle, valid }: { day: string; puzzle: Ladder; valid: string[] }) {
  const dictionary = useMemo(() => new Set(valid), [valid]);
  const [steps, setSteps] = useSaved("pp-ladder", `${day}~${puzzle.from}-${puzzle.to}`);
  const [typed, setTyped] = useState("");
  const [message, setMessage] = useState("");
  const chain = [puzzle.from, ...steps];
  const last = chain[chain.length - 1];
  const solved = last === puzzle.to;

  function add(e: React.FormEvent) {
    e.preventDefault();
    const w = typed.trim().toLowerCase();
    setTyped("");
    if (w.length !== 4) return setMessage("Use four letters");
    if (!isStep(last, w)) return setMessage("Change exactly one letter");
    if (!dictionary.has(w)) return setMessage("Not in the word list");
    if (chain.includes(w)) return setMessage("Already used");
    setMessage("");
    setSteps([...steps, w]);
  }

  async function share() {
    setMessage(await copyText(`Postpaper Word ladder ${dayNumber(day)}\n${steps.length} steps (par ${puzzle.par}) ${steps.length <= puzzle.par ? "🏆" : "✅"}\n${SITE_URL}/games/ladder`));
  }

  return (
    <div className="lad">
      <div className="lad-chain">
        {chain.map((w, i) => (
          <Word key={w} word={w} prev={chain[i - 1]} />
        ))}
        {!solved && (
          <>
            <p className="lad-dots" aria-hidden="true">
              ⋮
            </p>
            <Word word={puzzle.to} goal />
          </>
        )}
      </div>

      {solved ? (
        <div className="wd-end">
          <p className="wd-msg">{steps.length <= puzzle.par ? `Par or better: ${steps.length} steps.` : `Solved in ${steps.length} steps. Par is ${puzzle.par}.`}</p>
          <button type="button" className="btn primary" onClick={share}>
            Share your result
          </button>
          <p className="fine-print">{message || "A new ladder every day."}</p>
        </div>
      ) : (
        <>
          <form className="lad-form" onSubmit={add}>
            <input value={typed} onChange={(e) => setTyped(e.target.value.replace(/[^a-zA-Z]/g, "").slice(0, 4))} placeholder="Next word" aria-label="Next word" maxLength={4} autoCapitalize="none" autoComplete="off" spellCheck={false} />
            <button type="submit">Add</button>
            <button type="button" className="more" disabled={!steps.length} onClick={() => { setSteps(steps.slice(0, -1)); setMessage(""); }}>
              Undo
            </button>
          </form>
          <p className="wd-msg" role="status">
            {message}
          </p>
          <p className="fine-print">
            Steps so far: {steps.length}. Par is {puzzle.par}.
          </p>
        </>
      )}
    </div>
  );
}
