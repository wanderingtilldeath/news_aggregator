"use client";

import { useState } from "react";
import type { QuizQuestion } from "@/lib/api";
import { readLocal, writeLocal } from "@/lib/local";
import { safeURL } from "@/lib/safe";

export const MISSED_KEY = "pp-missed";
const MAX_MISSED = 30;

export function readMissed(raw: string): QuizQuestion[] {
  try {
    const list = JSON.parse(raw || "[]");
    return Array.isArray(list) ? list : [];
  } catch {
    return [];
  }
}

// Keeps the questions a reader got wrong in their browser, newest first, so they can be asked again.
function remember(q: QuizQuestion, missed: boolean) {
  const rest = readMissed(readLocal(MISSED_KEY)).filter((m) => m.question !== q.question);
  writeLocal(MISSED_KEY, JSON.stringify(missed ? [q, ...rest].slice(0, MAX_MISSED) : rest));
}

// The day's quiz, one tap per question: the chosen option and the right one are marked, the
// explanation appears, and the score is added up at the end. A wrong answer is remembered in the
// reader's browser (nothing is sent); with revision on, a right answer forgets the question.
export default function QuizPlayer({ questions, revision = false }: { questions: QuizQuestion[]; revision?: boolean }) {
  const [picked, setPicked] = useState<(number | null)[]>(() => questions.map(() => null));
  const answered = picked.filter((p) => p !== null).length;
  const score = picked.filter((p, i) => p === questions[i].answer).length;

  function pick(i: number, j: number) {
    setPicked((p) => p.map((v, k) => (k === i ? j : v)));
    const right = j === questions[i].answer;
    if (!right) remember(questions[i], true);
    else if (revision) remember(questions[i], false);
  }

  return (
    <>
      {questions.map((q, i) => {
        const choice = picked[i];
        const href = safeURL(q.url);
        return (
          <section className="q" key={i}>
            <h2>
              {i + 1}. {q.question}
            </h2>
            {q.options.map((o, j) => {
              const cls = choice === null ? "opt" : j === q.answer ? "opt right" : j === choice ? "opt wrong" : "opt";
              return (
                <button key={j} type="button" className={cls} disabled={choice !== null} onClick={() => pick(i, j)}>
                  {o}
                </button>
              );
            })}
            {choice !== null && (
              <p className="explain">
                {choice === q.answer ? "Correct. " : "Not quite. "}
                {q.explanation}{" "}
                {href && (
                  <a href={href} target="_blank" rel="noopener noreferrer">
                    Read the story
                  </a>
                )}
              </p>
            )}
          </section>
        );
      })}
      {answered === questions.length && (
        <p className="score" role="status">
          You scored {score} out of {questions.length}.
        </p>
      )}
    </>
  );
}
