"use client";

import { useSyncExternalStore } from "react";
import { BOOKMARKS_KEY, parseBookmarks, toggled } from "@/lib/bookmarks";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";

// Saves a story to the reader's bookmarks (this browser only) or takes it out. compact is the small round
// icon over a card; otherwise it is the labelled button on the story page.
export default function BookmarkButton({ id, title, source, time, compact }: { id: string; title: string; source: string; time: string; compact?: boolean }) {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(BOOKMARKS_KEY), () => "");
  const saved = parseBookmarks(raw).some((b) => b.id === id);
  const label = saved ? "Remove from your bookmarks" : "Bookmark to read later";

  function press() {
    writeLocal(BOOKMARKS_KEY, JSON.stringify(toggled(parseBookmarks(readLocal(BOOKMARKS_KEY)), { id, title, source, time, at: Date.now() })));
  }

  return (
    <button type="button" className={`bookmark${compact ? " compact" : ""}${saved ? " saved" : ""}`} aria-pressed={saved} onClick={press} title={label} aria-label={compact ? `${label}: ${title}` : undefined}>
      <svg width="16" height="16" viewBox="0 0 24 24" fill={saved ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true">
        <path d="M6 3h12v18l-6-4-6 4V3Z" />
      </svg>
      {!compact && <span>{saved ? "Saved" : "Save for later"}</span>}
    </button>
  );
}
