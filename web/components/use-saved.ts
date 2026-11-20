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

// Copies a result to the clipboard; returns the message to show.
export async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text);
    return "Copied. Paste it to share.";
  } catch {
    return "Could not copy the result.";
  }
}
