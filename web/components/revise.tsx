"use client";

import { useState, useSyncExternalStore } from "react";
import QuizPlayer, { MISSED_KEY, readMissed } from "./quiz-player";
import { readLocal, subscribeLocal } from "@/lib/local";

// A fixed set of questions for one sitting: the list in storage shrinks as answers come right, and
// the player must not reshuffle under the reader, so the set is taken once when it opens.
function Session() {
  const [questions] = useState(() => readMissed(readLocal(MISSED_KEY)).slice(0, 5));
  return <QuizPlayer questions={questions} revision />;
}

// The questions this reader missed on earlier days, asked again until they get them right. They
// live in the reader's browser only, so the list is empty on a new device.
export default function Revise() {
  const raw = useSyncExternalStore(
    subscribeLocal,
    () => readLocal(MISSED_KEY),
    () => "",
  );
  const [opened, setOpened] = useState(false);
  const count = readMissed(raw).length;
  if (!count && !opened) return null;

  return (
    <details className="revise" onToggle={(e) => e.currentTarget.open && setOpened(true)}>
      <summary>{count ? `Revise the ${count} question${count === 1 ? "" : "s"} you missed` : "Nothing left to revise"}</summary>
      {opened && <Session />}
    </details>
  );
}
