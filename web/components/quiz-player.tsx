"use client";

import { useState } from "react";
import type { QuizQuestion } from "@/lib/api";
import { safeURL } from "@/lib/safe";

// The day's quiz, one tap per question: the chosen option and the right one are marked, the
// explanation appears, and the score is added up at the end. Nothing is stored or sent.
export default function QuizPlayer({ questions }: { questions: QuizQuestion[] }) {
  const [picked, setPicked] = useState<(number | null)[]>(() => questions.map(() => null));
  const answered = picked.filter((p) => p !== null).length;
  const score = picked.filter((p, i) => p === questions[i].answer).length;

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
                <button key={j} type="button" className={cls} disabled={choice !== null} onClick={() => setPicked((p) => p.map((v, k) => (k === i ? j : v)))}>
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
