"use client";

import { useSyncExternalStore } from "react";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// A list a game keeps in the reader's browser for one puzzle ("id", a tab, then one item per line); a different
// puzzle id starts a fresh list. Nothing is sent anywhere.
export function useSaved(key: string, id: string): [string[], (items: string[]) => void] {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(key), () => "");
  const [savedId, saved = ""] = raw.split("\t");
  return [savedId === id && saved ? saved.split("\n") : [], (items) => writeLocal(key, `${id}\t${items.join("\n")}`)];
}

// Ids of the practice puzzles already played (kept in the browser, newest last, at most 2,000), so a replay can
// prefer one the reader has not seen.
export const readSeen = (key: string) => new Set(readLocal(key).split("\n").filter(Boolean));
export function remember(key: string, id: string) {
  writeLocal(key, [...readSeen(key), id].slice(-2000).join("\n"));
}
// A random item of the pool that has not been played yet; when all have, the memory is cleared and it starts over.
export function pickFresh(pool: string[], key: string) {
  const seen = readSeen(key);
  const fresh = pool.filter((w) => !seen.has(w));
  const pick = (fresh.length ? fresh : pool)[Math.floor(Math.random() * (fresh.length || pool.length))];
  if (!fresh.length) writeLocal(key, "");
  remember(key, pick);
  return pick;
}

// Copies a result to the clipboard; returns the message to show.
export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    return "Copied. Paste it to share.";
  } catch {
    return "Could not copy the result.";
  }
}
