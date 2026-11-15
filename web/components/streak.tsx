"use client";

import { useEffect, useSyncExternalStore } from "react";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

const KEY = "pp-streak";
const day = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;

// Counts the days in a row a reader got to the end of the edition, kept in their browser. It shows
// from the second day on.
export default function Streak() {
  const raw = useSyncExternalStore(
    subscribeLocal,
    () => readLocal(KEY),
    () => "",
  );

  useEffect(() => {
    const today = new Date();
    const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1);
    const [last, count] = readLocal(KEY).split("|");
    if (last === day(today)) return;
    writeLocal(KEY, `${day(today)}|${last === day(yesterday) ? (parseInt(count, 10) || 0) + 1 : 1}`);
  }, []);

  const count = parseInt(raw.split("|")[1], 10) || 0;
  if (count < 2) return null;

  return (
    <p className="streak" role="status">
      <b>{count}-day reading streak.</b> Come back tomorrow to keep it.
    </p>
  );
}
