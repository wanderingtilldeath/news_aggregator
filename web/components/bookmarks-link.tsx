"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { BOOKMARKS_KEY, parseBookmarks } from "@/lib/bookmarks";
import { readLocal, subscribeLocal } from "@/lib/local";

// A bookmark icon at the right of the sections bar, on every page, that opens the saved stories. The number beside it
// is how many are saved; it changes the moment one is saved or removed. It reads this browser's storage, so it shows
// no number until the page is up.
export default function BookmarksLink() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(BOOKMARKS_KEY), () => "");
  const count = parseBookmarks(raw).length;
  const label = count ? `Bookmarks, ${count} saved ${count === 1 ? "story" : "stories"}` : "Bookmarks";

  return (
    <Link href="/bookmarks" className="nav-bookmarks" aria-label={label} title="Bookmarks">
      <svg width="18" height="18" viewBox="0 0 24 24" fill={count ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2" strokeLinejoin="round" aria-hidden="true">
        <path d="M6 3h12v18l-6-4-6 4V3Z" />
      </svg>
      {count > 0 && (
        <span className="nav-badge" aria-hidden="true">
          {count > 99 ? "99+" : count}
        </span>
      )}
    </Link>
  );
}
