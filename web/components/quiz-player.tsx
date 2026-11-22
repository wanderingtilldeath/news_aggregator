"use client";

import Link from "next/link";
import { useState } from "react";
import type { QuizQuestion } from "@/lib/api";
import { readLocal, writeLocal } from "@/lib/local";
import { safeURL } from "@/lib/safe";

export const MISSED_KEY = "pp-missed";
const MAX_MISSED = 30;
const LETTERS = "ABCDEFGH";

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

function verdict(score: number, total: number) {
  if (score === total) return "A clean sweep. You are fully caught up.";
  if (score >= Math.ceil(total * 0.6)) return "A solid read of the day. The misses are saved to revise below.";
  return "Worth another look at the stories. The misses are saved to revise below.";
}

// The day's quiz, one question at a time: a progress bar on top, lettered options, and after a tap the chosen
// and the right option are marked and the explanation appears before the next question. The score and a recap
// of every answer close the sitting. A wrong answer is remembered in the reader's browser (nothing is sent);
// with revision on, a right answer forgets the question.
export default function QuizPlayer({ questions, revision = false }: { questions: QuizQuestion[]; revision?: boolean }) {
  const [picked, setPicked] = useState<(number | null)[]>(() => questions.map(() => null));
  const [at, setAt] = useState(0);
  const [round, setRound] = useState(0);

  const total = questions.length;
  const done = at >= total;
  const score = picked.filter((p, i) => p === questions[i].answer).length;

  function pick(j: number) {
    setPicked((p) => p.map((v, k) => (k === at ? j : v)));
    const right = j === questions[at].answer;
    if (!right) remember(questions[at], true);
    else if (revision) remember(questions[at], false);
  }

  function again() {
    setPicked(questions.map(() => null));
    setAt(0);
    setRound((r) => r + 1);
  }

  if (!total) return null;

  if (done) {
    return (
      <div className="quiz" key={round}>
        <div className="quiz-result" role="status">
          <p className="quiz-eyebrow">Your score</p>
          <p className="quiz-score">
            {score}
            <span> / {total}</span>
          </p>
          <p className="quiz-verdict">{verdict(score, total)}</p>
          <div className="quiz-actions">
            <button type="button" className="btn primary" onClick={again}>
              Try again
            </button>
            <Link className="btn" href="/flashcards">
              Study flashcards
            </Link>
          </div>
        </div>
        <ol className="quiz-recap" aria-label="Your answers">
          {questions.map((q, i) => {
            const right = picked[i] === q.answer;
            const href = safeURL(q.url);
            return (
              <li key={i} className={right ? "right" : "wrong"}>
                <span className="quiz-mark" aria-hidden="true">
                  {right ? "✓" : "✕"}
                </span>
                <div>
                  <p className="quiz-recap-q">{q.question}</p>
                  <p className="quiz-recap-a">
                    {right ? "You answered" : "Right answer"}: <b>{q.options[q.answer]}</b>
                    {!right && picked[i] !== null && <> · you chose {q.options[picked[i] as number]}</>}
                    {href && (
                      <>
                        {" · "}
                        <a href={href} target="_blank" rel="noopener noreferrer">
                          Read the story
                        </a>
                      </>
                    )}
                  </p>
                </div>
              </li>
            );
          })}
        </ol>
      </div>
    );
  }

  const q = questions[at];
  const choice = picked[at];
  const href = safeURL(q.url);
  const last = at === total - 1;

  return (
    <div className="quiz" key={round}>
      <div className="quiz-top">
        <p className="quiz-eyebrow">
          Question {at + 1} of {total}
        </p>
        <p className="quiz-eyebrow quiz-running" aria-live="polite">
          {picked.filter((p, i) => p !== null && p === questions[i].answer).length} correct
        </p>
      </div>
      <div className="quiz-bar" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={at + (choice === null ? 0 : 1)}>
        {questions.map((_, i) => {
          const p = picked[i];
          const state = p === null ? (i === at ? "now" : "") : p === questions[i].answer ? "right" : "wrong";
          return <i key={i} className={state} />;
        })}
      </div>

      <h2 className="quiz-q">{q.question}</h2>
      <div className="quiz-options" role="group" aria-label="Answers">
        {q.options.map((o, j) => {
          const state = choice === null ? "" : j === q.answer ? "right" : j === choice ? "wrong" : "dim";
          return (
            <button key={j} type="button" className={`quiz-opt ${state}`} disabled={choice !== null} onClick={() => pick(j)}>
              <span className="quiz-letter" aria-hidden="true">
                {choice !== null && j === q.answer ? "✓" : choice === j ? "✕" : LETTERS[j]}
              </span>
              <span>{o}</span>
            </button>
          );
        })}
      </div>

      {choice !== null && (
        <div className={`quiz-explain ${choice === q.answer ? "right" : "wrong"}`} role="status">
          <p className="quiz-eyebrow">{choice === q.answer ? "Correct" : "Not quite"}</p>
          <p>{q.explanation}</p>
          {href && (
            <a href={href} target="_blank" rel="noopener noreferrer">
              Read the story
            </a>
          )}
        </div>
      )}

      <div className="quiz-actions">
        <button type="button" className="btn primary" disabled={choice === null} onClick={() => setAt(at + 1)}>
          {last ? "See your score" : "Next question"}
        </button>
      </div>
    </div>
  );
}
