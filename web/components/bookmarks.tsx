"use client";

import Link from "next/link";
import { useSyncExternalStore } from "react";
import { BOOKMARKS_KEY, parseBookmarks, type Bookmark } from "@/lib/bookmarks";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { hostName } from "@/lib/view";

// The bookmarks page: the stories the reader saved, newest saved first, each with a way to take it out. It
// reads this browser's storage.
export default function Bookmarks() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(BOOKMARKS_KEY), () => "");
  const list = parseBookmarks(raw);
  const save = (next: Bookmark[]) => writeLocal(BOOKMARKS_KEY, JSON.stringify(next));

  if (!list.length) {
    return <p className="empty">No bookmarks yet. Press the bookmark on a story card or a story page to keep it here. They stay in this browser, or follow your account when you are logged in.</p>;
  }

  return (
    <>
      <div className="printbar">
        <button type="button" onClick={() => window.confirm("Remove all your bookmarks?") && save([])}>
          Clear all
        </button>
      </div>
      <ul className="bm-list">
        {list.map((b) => (
          <li key={b.id}>
            <div>
              <Link href={`/story/${encodeURIComponent(b.id)}`}>{b.title}</Link>
              <span>
                {hostName(b.source)} · saved {new Date(b.at).toLocaleDateString("en-IN", { day: "numeric", month: "short" })}
              </span>
            </div>
            <button type="button" className="nb-remove" onClick={() => save(list.filter((x) => x.id !== b.id))} aria-label={`Remove ${b.title}`}>
              ✕
            </button>
          </li>
        ))}
      </ul>
    </>
  );
}
