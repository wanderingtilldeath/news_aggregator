"use client";

import Link from "next/link";
import { useState, useSyncExternalStore } from "react";
import { BOOKMARKS_KEY, filterBookmarks, groupBySaved, parseBookmarks, type Bookmark } from "@/lib/bookmarks";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { ACCOUNT_KEY } from "@/lib/notebook";
import { ago, cleanTitle, hostName } from "@/lib/view";

// The time, read outside the component (which must not call impure functions while it renders).
const clock = () => Date.now();

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// Past this many bookmarks a search box appears above the list.
const SEARCH_FROM = 8;

// The bookmarks page, laid out like the notepad: a header with the count and the actions, one line saying where the
// bookmarks live, then the stories as cards, grouped by when they were saved (newest first). It reads this browser's
// storage; a logged-in account's bookmarks also follow it to other devices (see lib/account-sync.ts).
export default function Bookmarks() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(BOOKMARKS_KEY), () => "");
  const account = useSyncExternalStore(subscribeLocal, () => readLocal(ACCOUNT_KEY), () => "");
  const ready = useSyncExternalStore(() => () => {}, () => true, () => false);
  const [query, setQuery] = useState("");

  const list = parseBookmarks(raw);
  const save = (next: Bookmark[]) => writeLocal(BOOKMARKS_KEY, JSON.stringify(next));
  const clear = () => {
    if (window.confirm(account ? "Remove all your bookmarks, here and on your account?" : "Remove all your bookmarks?")) save([]);
  };

  if (!ready) {
    return (
      <header className="np-head">
        <h1>Bookmarks</h1>
      </header>
    );
  }

  const now = clock();
  const shown = filterBookmarks(list, query);
  const groups = groupBySaved(shown, now);
  const sources = new Set(list.map((b) => hostName(b.source))).size;

  return (
    <>
      <header className="np-head">
        <div>
          <h1>Bookmarks</h1>
          {list.length > 0 && (
            <p className="np-count">
              {plural(list.length, "saved story", "saved stories")} from {plural(sources, "publisher", "publishers")}
            </p>
          )}
        </div>
        {list.length > 0 && (
          <div className="np-tools" role="group" aria-label="Bookmark actions">
            <button type="button" onClick={() => window.print()} title="Print or save as PDF">
              Print
            </button>
            <button type="button" className="danger" onClick={clear}>
              Clear all
            </button>
          </div>
        )}
      </header>

      {account ? (
        <div className="np-strip ok">
          <span>
            <b>Synced</b> · logged in as {account} · your bookmarks follow you to your other devices
          </span>
          <Link href="/account">Account</Link>
        </div>
      ) : (
        <div className="np-strip">
          <span>
            <b>Saved on this device.</b> Log in to keep your bookmarks on every device; clearing your browser data removes them until then.
          </span>
          <Link href="/login">Log in</Link>
          <Link href="/signup">Sign up</Link>
        </div>
      )}

      {list.length === 0 ? (
        <div className="np-empty">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" aria-hidden="true">
            <path d="M6 3h12v18l-6-4-6 4V3Z" />
          </svg>
          <h2>No bookmarks yet</h2>
          <p>Press the bookmark on a story card or a story page to keep it here for later.</p>
          <Link href="/" className="btn primary">
            Read today&rsquo;s stories
          </Link>
        </div>
      ) : (
        <>
          {list.length >= SEARCH_FROM && (
            <div className="np-filter">
              <label className="sr-only" htmlFor="bm-q">
                Search your bookmarks
              </label>
              <input id="bm-q" type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search your bookmarks" autoComplete="off" />
            </div>
          )}
          {shown.length === 0 && <p className="empty">Nothing matches.</p>}
          {groups.map((g) => (
            <section key={g.label} className="bm-group" aria-label={g.label}>
              <h2>
                {g.label} <span>{g.items.length}</span>
              </h2>
              <div className="bm-grid">
                {g.items.map((b) => {
                  const published = Number.isNaN(Date.parse(b.time)) ? "" : `${ago(b.time, now)} · `;
                  return (
                    <article key={b.id} className="bm-card">
                      <p className="bm-source">{hostName(b.source)}</p>
                      <h3>
                        <Link href={`/story/${encodeURIComponent(b.id)}`}>{cleanTitle(b.title)}</Link>
                      </h3>
                      <p className="bm-meta">
                        {published}saved {new Date(b.at).toLocaleDateString("en-IN", { day: "numeric", month: "short" })}
                      </p>
                      <div className="np-actions">
                        <Link href={`/story/${encodeURIComponent(b.id)}`}>Read</Link>
                        <button type="button" onClick={() => save(list.filter((x) => x.id !== b.id))} aria-label={`Remove: ${b.title}`}>
                          Remove
                        </button>
                      </div>
                    </article>
                  );
                })}
              </div>
            </section>
          ))}
        </>
      )}
    </>
  );
}
