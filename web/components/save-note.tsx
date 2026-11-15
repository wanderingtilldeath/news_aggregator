"use client";

import { useSyncExternalStore } from "react";
import { NOTEBOOK_KEY, noteId, parseNotes, toggled } from "@/lib/notebook";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// A star beside a sentence on a story page: one press saves it to the reader's notebook, another
// takes it out. The notebook lives in this browser only.
export default function SaveNote({ story, title, text }: { story: string; title: string; text: string }) {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const id = noteId(story, text);
  const saved = parseNotes(raw).some((n) => n.id === id);

  function press() {
    const next = toggled(parseNotes(readLocal(NOTEBOOK_KEY)), { id, text, story, title, at: Date.now() });
    writeLocal(NOTEBOOK_KEY, JSON.stringify(next));
  }

  return (
    <button type="button" className={saved ? "note-btn saved" : "note-btn"} aria-pressed={saved} onClick={press} title={saved ? "Remove from your notebook" : "Save to your notebook"}>
      <span aria-hidden="true">{saved ? "★" : "☆"}</span>
      <span className="sr-only">{saved ? "Remove from your notebook" : "Save to your notebook"}</span>
    </button>
  );
}
