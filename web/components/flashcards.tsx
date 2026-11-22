"use client";

import Link from "next/link";
import { useState, useSyncExternalStore } from "react";
import type { DeckCard } from "@/lib/api";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

const KEY = "pp-cards";
const DAY = 24 * 60 * 60 * 1000;
// Days until a card is asked again, by the box it sits in: a right answer moves it up a box, a
// miss sends it back to the start. This is the Leitner system, kept in the reader's browser.
const GAPS = [0, 1, 3, 7, 14, 30];
const SESSION = 12;

type Progress = Record<string, { box: number; due: number }>;

function parse(raw: string): Progress {
  try {
    const value = JSON.parse(raw || "{}");
    return value && typeof value === "object" && !Array.isArray(value) ? value : {};
  } catch {
    return {};
  }
}

const noop = () => () => {};
// When this page was opened: the counts below are worked out from it, so they stay the same between
// renders. (Server renders get 0, so nothing is baked into the static page.)
const OPENED = Date.now();

// Flashcards from the Prelims facts of the most important recent stories. The reader sees a question,
// reveals the answer and says whether they knew it; cards they missed come back soon and cards they
// know come back after longer and longer gaps. Nothing is sent anywhere.
export default function Flashcards({ cards }: { cards: DeckCard[] }) {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(KEY), () => "");
  const now = useSyncExternalStore(noop, () => OPENED, () => 0);
  const progress = parse(raw);

  const fresh = cards.filter((c) => !progress[c.id]).length;
  const due = cards.filter((c) => progress[c.id] && progress[c.id].due <= now).length;
  const known = cards.filter((c) => (progress[c.id]?.box ?? 0) >= 3).length;

  const [queue, setQueue] = useState<DeckCard[]>([]);
  const [index, setIndex] = useState(0);
  const [shown, setShown] = useState(false);
  const [reviewed, setReviewed] = useState(0);
  const [started, setStarted] = useState(false);
  const [retried, setRetried] = useState<string[]>([]);

  function start() {
    const stamp = Date.now();
    const saved = parse(readLocal(KEY));
    const dueNow = cards.filter((c) => saved[c.id] && saved[c.id].due <= stamp).sort((a, b) => saved[a.id].due - saved[b.id].due);
    const brandNew = cards.filter((c) => !saved[c.id]);
    setQueue([...dueNow, ...brandNew].slice(0, SESSION));
    setIndex(0);
    setShown(false);
    setReviewed(0);
    setRetried([]);
    setStarted(true);
  }

  function grade(card: DeckCard, knew: boolean) {
    const saved = parse(readLocal(KEY));
    const box = knew ? Math.min((saved[card.id]?.box ?? 0) + 1, GAPS.length - 1) : 0;
    const stamp = Date.now();
    saved[card.id] = { box, due: knew ? stamp + GAPS[box] * DAY : stamp + 10 * 60 * 1000 };
    writeLocal(KEY, JSON.stringify(saved));

    // A missed card is asked once more at the end of this sitting.
    if (!knew && !retried.includes(card.id)) {
      setQueue((q) => [...q, card]);
      setRetried((r) => [...r, card.id]);
    }
    setReviewed((n) => n + 1);
    setIndex((i) => i + 1);
    setShown(false);
  }

  function reset() {
    if (window.confirm("Forget all your flashcard progress on this device?")) {
      writeLocal(KEY, "{}");
      setStarted(false);
    }
  }

  if (!cards.length) {
    return (
      <div className="empty-card">
        <h2>No flashcards yet</h2>
        <p>They are written for the most important UPSC stories of each day, so check back after the next edition.</p>
        <Link className="btn primary" href="/">
          Read today&rsquo;s news
        </Link>
      </div>
    );
  }

  const card = queue[index];
  const finished = started && !card;
  const ready = fresh + due;
  const total = queue.length;

  return (
    <div className="flash">
      <dl className="flash-stats" aria-live="polite">
        <div>
          <dt>New</dt>
          <dd>{fresh}</dd>
        </div>
        <div>
          <dt>Due</dt>
          <dd>{due}</dd>
        </div>
        <div>
          <dt>Well known</dt>
          <dd>{known}</dd>
        </div>
        <div>
          <dt>In the deck</dt>
          <dd>{cards.length}</dd>
        </div>
      </dl>

      {!started || finished ? (
        <div className="flash-card flash-start">
          <p className="quiz-eyebrow">{finished ? "Sitting complete" : "Today\u2019s deck"}</p>
          <h2 className="flash-q">
            {finished
              ? `You went through ${reviewed} card${reviewed === 1 ? "" : "s"}.`
              : ready
                ? `${Math.min(ready, SESSION)} cards are ready for you.`
                : "Nothing is due right now."}
          </h2>
          <p className="flash-note">
            {finished ? "Missed cards come back within the hour, and the others after longer gaps." : "Cards you miss come back soon, and the ones you know come back after longer and longer gaps."}
          </p>
          <button type="button" className="btn primary big" onClick={start} disabled={ready === 0}>
            {ready === 0 ? "Come back later" : finished ? "Study more" : "Start studying"}
          </button>
        </div>
      ) : (
        <div className="flash-card" role="group" aria-label={`Card ${Math.min(index + 1, total)} of ${total}`}>
          <div className="quiz-top">
            <p className="quiz-eyebrow">
              Card {index + 1} of {total}
            </p>
            <p className="quiz-eyebrow quiz-running">{reviewed} reviewed</p>
          </div>
          <div className="quiz-bar" role="progressbar" aria-valuemin={0} aria-valuemax={total} aria-valuenow={index}>
            {queue.map((_, i) => (
              <i key={i} className={i < index ? "right" : i === index ? "now" : ""} />
            ))}
          </div>
          <h2 className="flash-q">{card.q}</h2>
          {shown ? (
            <>
              <div className="quiz-explain right">
                <p className="quiz-eyebrow">Answer</p>
                <p className="flash-a">{card.a}</p>
                <Link href={`/story/${encodeURIComponent(card.story)}`}>From: {card.title}</Link>
              </div>
              <div className="quiz-actions">
                <button type="button" className="btn" onClick={() => grade(card, false)}>
                  Missed it
                </button>
                <button type="button" className="btn primary" onClick={() => grade(card, true)}>
                  Got it
                </button>
              </div>
            </>
          ) : (
            <div className="quiz-actions">
              <button type="button" className="btn primary big" onClick={() => setShown(true)}>
                Show answer
              </button>
            </div>
          )}
        </div>
      )}

      <p className="flash-foot">
        Your progress stays in this browser.{" "}
        <button type="button" className="linklike" onClick={reset}>
          Reset it
        </button>
      </p>
    </div>
  );
}
