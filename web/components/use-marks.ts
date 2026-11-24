"use client";

import { useSyncExternalStore } from "react";
import { MARKS_KEY, parseMarks, type Mark } from "@/lib/map-marks";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { MAPS_KEY } from "@/lib/maps";

const SEP = "\u0001";

// The reader's marks (starred places and notes) and a way to save them. The earlier single list (key pp-maps) is read until
// the first save and then cleared, so it is not read back when the account sync leaves the new key empty.
export function useMarks(): [Mark[], (next: Mark[]) => void] {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(MARKS_KEY) + SEP + readLocal(MAPS_KEY), () => SEP);
  const [now, legacy] = raw.split(SEP);
  const save = (next: Mark[]) => {
    writeLocal(MARKS_KEY, JSON.stringify(next));
    if (readLocal(MAPS_KEY)) writeLocal(MAPS_KEY, "");
  };
  return [parseMarks(now, legacy), save];
}
