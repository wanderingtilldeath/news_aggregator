"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { NOTEBOOK_KEY, parseNotes } from "@/lib/notebook";
import { readLocal, subscribeLocal } from "@/lib/local";

// The notebook's door in the top bar, on every page: an icon with the number of saved sentences, which changes the
// moment one is saved or removed. It reads this browser's storage, so it shows no number until the page is up.
export default function NotebookLink() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(NOTEBOOK_KEY), () => "");
  const count = parseNotes(raw).length;
  const label = count ? `Your notebook, ${count} saved ${count === 1 ? "sentence" : "sentences"}` : "Your notebook";

  return (
    <Link href="/notebook" className="nav-notebook" aria-label={label} title={label}>
      <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
        <path d="M6 3h11a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6V3Z" />
        <path d="M6 3v18M10 8h5M10 12h5" />
      </svg>
      {count > 0 && (
        <span className="nav-badge" aria-hidden="true">
          {count > 99 ? "99+" : count}
        </span>
      )}
    </Link>
  );
}
