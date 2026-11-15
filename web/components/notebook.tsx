"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { NOTEBOOK_KEY, parseNotes, type Note } from "@/lib/notebook";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// The notebook page: the saved sentences grouped by story, newest story first, with a way to take
// one out, copy everything as text, print or save as PDF, or clear it. It reads this browser's storage.
export default function Notebook() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const notes = parseNotes(raw);

  const groups = new Map<string, { title: string; notes: Note[] }>();
  for (const n of notes) {
    if (!groups.has(n.story)) groups.set(n.story, { title: n.title, notes: [] });
    groups.get(n.story)!.notes.push(n);
  }

  const save = (list: Note[]) => writeLocal(NOTEBOOK_KEY, JSON.stringify(list));
  const copy = () => {
    const text = [...groups.values()].map((g) => `${g.title}\n${g.notes.map((n) => `- ${n.text}`).join("\n")}`).join("\n\n");
    void navigator.clipboard?.writeText(text);
  };

  if (!notes.length) {
    return (
      <p className="empty">
        Your notebook is empty. Press the star beside a sentence on a story page to save it here. Everything you save stays in this browser.
      </p>
    );
  }

  return (
    <>
      <div className="printbar notebook-actions">
        <button type="button" onClick={copy}>
          Copy as text
        </button>
        <button type="button" onClick={() => window.print()}>
          Print or save as PDF
        </button>
        <button type="button" onClick={() => window.confirm("Clear your whole notebook?") && save([])}>
          Clear
        </button>
      </div>
      {[...groups.entries()].map(([story, g]) => (
        <section key={story} className="nb-story">
          <h2>
            <Link href={`/story/${encodeURIComponent(story)}`}>{g.title}</Link>
          </h2>
          <ul>
            {g.notes.map((n) => (
              <li key={n.id}>
                {n.text}{" "}
                <button type="button" className="nb-remove" onClick={() => save(notes.filter((x) => x.id !== n.id))} aria-label="Remove this note">
                  ✕
                </button>
              </li>
            ))}
          </ul>
        </section>
      ))}
    </>
  );
}
