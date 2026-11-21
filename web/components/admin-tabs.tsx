"use client";

import { useEffect, useState } from "react";
import { loadContent, loadSubscribers, removeSubscriber, type AdminResult } from "@/app/actions";
import type { Analytics, ContentData, SubscriberList } from "@/lib/api";
import { ago } from "@/lib/view";
import { Bars, Tile, Windows, sum } from "./analytics-board";

// The most mails a day Brevo's free plan allows, shared by the daily edition and the confirmation mails.
const MAIL_LIMIT = 300;

const hostOf = (url: string) => {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
};

function Problem({ res }: { res: Extract<AdminResult<unknown>, { ok: false }> }) {
  return (
    <p className="note err" role="status">
      {res.signedOut ? "Your session has ended. Reload the page to sign in again." : res.message}
    </p>
  );
}

// The daily email: how many went out, how many of today's readers are still waiting, and the mail quota used.
export function EmailsTab({ data, days, onDays }: { data: Analytics; days: number; onDays: (w: number) => void }) {
  const today = data.daily[data.daily.length - 1];
  const sentToday = today?.sent ?? 0;
  const notYet = Math.max(0, data.subscribers.active - sentToday);
  // A confirmation mail goes out for each new address, so signups stand in for them.
  const mailsToday = sentToday + (today?.signups ?? 0);
  const recent = [...data.daily].reverse().slice(0, 10);

  return (
    <div className="an">
      <Windows days={days} onDays={onDays} />
      <div className="an-tiles">
        <Tile label="Sent today" value={sentToday} note="the daily edition" />
        <Tile label="Not yet sent today" value={notYet} note="active readers minus sent; 0 once the send finishes" />
        <Tile label="Mails today, about" value={`${mailsToday} of ${MAIL_LIMIT}`} note="edition plus a confirmation per signup" />
        <Tile label="Active readers" value={data.subscribers.active} />
        <Tile label={`Sent, ${days} days`} value={sum(data.daily.map((d) => d.sent))} />
      </div>

      <h2>Sent and signups per day</h2>
      <Bars days={data.daily} a={(d) => d.sent} b={(d) => d.signups} labelA="sent" labelB="signups" />
      <p className="an-legend">
        <span className="dot a" /> edition sent <span className="dot b" /> signups (one confirmation mail each)
      </p>

      <h2>Last 10 days</h2>
      <div className="an-scroll">
        <table className="an-table">
          <thead>
            <tr>
              <th>Day</th>
              <th>Edition sent</th>
              <th>Signups</th>
              <th>Mails, about</th>
            </tr>
          </thead>
          <tbody>
            {recent.map((d) => (
              <tr key={d.day}>
                <td>{d.day}</td>
                <td>{d.sent}</td>
                <td>{d.signups}</td>
                <td>
                  {d.sent + d.signups}
                  {d.sent + d.signups > MAIL_LIMIT && <b className="an-over"> over the limit</b>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="an-note">
        Failed sends are not recorded here: see the &ldquo;Email the daily digest&rdquo; step of the crawl job on GitHub. Opens and clicks are in Brevo.
      </p>
    </div>
  );
}

// The news side: how many articles came in, how far the pipeline has got, and where the stories come from.
export function ContentTab() {
  const [days, setDays] = useState(30);
  const [state, setState] = useState<{ days: number; now: number; res: AdminResult<ContentData> } | null>(null);

  useEffect(() => {
    let live = true;
    void loadContent(days).then((res) => {
      if (live) setState({ days, now: Date.now(), res });
    });
    return () => {
      live = false;
    };
  }, [days]);

  if (!state || state.days !== days) {
    return (
      <div className="an">
        <Windows days={days} onDays={setDays} />
        <p className="an-note">Loading…</p>
      </div>
    );
  }
  if (!state.res.ok) return <Problem res={state.res} />;

  const c = state.res.data;
  const now = state.now;
  const total = sum(c.daily.map((d) => d.articles));
  const w = c.waiting;

  return (
    <div className="an">
      <Windows days={days} onDays={setDays} />
      <div className="an-tiles">
        <Tile label="Newest article" value={c.latest_article ? ago(c.latest_article, now) : "none"} note="when it was first seen" />
        <Tile label={`Articles, ${days} days`} value={total} />
        <Tile label="Waiting for a gist" value={w.summary} note="last 7 days" />
        <Tile label="Waiting for enrichment" value={w.enrichment} note="why it matters, importance, tags" />
        <Tile label="Waiting for the long read" value={w.detail} />
        <Tile label="Waiting for an image" value={w.image} note="source-page lookups" />
      </div>

      <h2>Articles per day</h2>
      <Bars days={c.daily} a={(d) => d.articles} labelA="articles" />

      <div className="an-two">
        <div>
          <h2>Last 7 days by category</h2>
          <ul className="an-list">
            {c.categories.length === 0 && <li>No articles yet.</li>}
            {c.categories.map((x) => (
              <li key={x.category}>
                <span>{x.category}</span>
                <b>{x.count}</b>
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h2>Top sources, 7 days</h2>
          <ul className="an-list">
            {c.sources.length === 0 && <li>No articles yet.</li>}
            {c.sources.map((x) => (
              <li key={x.source}>
                <span>{x.source}</span>
                <b>{x.count}</b>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <h2>Latest crawls</h2>
      <div className="an-scroll">
        <table className="an-table">
          <thead>
            <tr>
              <th>Site</th>
              <th>Started</th>
              <th>Pages</th>
              <th>Waiting</th>
              <th>Failed</th>
              <th>Blocked</th>
            </tr>
          </thead>
          <tbody>
            {c.crawls.length === 0 && (
              <tr>
                <td colSpan={6}>No crawls kept (they are pruned after 3 days).</td>
              </tr>
            )}
            {c.crawls.map((x) => (
              <tr key={x.id}>
                <td>
                  {hostOf(x.seed)}
                  {x.category && <span className="an-note"> · {x.category}</span>}
                </td>
                <td>{ago(x.created_at, now)}</td>
                <td>{x.total}</td>
                <td>{x.done ? "done" : x.pending}</td>
                <td>{x.failed}</td>
                <td>{x.blocked}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

// The subscribers: search by address and remove one. It shows personal data, so it is for admins only.
export function SubscribersTab() {
  const [q, setQ] = useState("");
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [nonce, setNonce] = useState(0);
  const [state, setState] = useState<{ key: string; res: AdminResult<SubscriberList> } | null>(null);
  const [note, setNote] = useState("");

  const key = `${query}|${page}|${nonce}`;
  useEffect(() => {
    let live = true;
    void loadSubscribers(query, page).then((res) => {
      if (live) setState({ key, res });
    });
    return () => {
      live = false;
    };
  }, [query, page, nonce, key]);

  async function remove(id: number, email: string) {
    if (!window.confirm(`Remove ${email}? They stop getting the briefing and the record is deleted. This cannot be undone.`)) return;
    const res = await removeSubscriber(id);
    setNote(res.ok ? `Removed ${email}.` : res.signedOut ? "Your session has ended. Reload the page to sign in again." : res.message);
    setNonce((n) => n + 1);
  }

  const res = state?.key === key ? state.res : null;
  const list = res?.ok ? res.data : null;
  const pages = list ? Math.max(1, Math.ceil(list.total / list.page_size)) : 1;

  return (
    <div className="an">
      <form
        className="an-search"
        onSubmit={(e) => {
          e.preventDefault();
          setPage(1);
          setQuery(q.trim());
        }}
      >
        <label className="sr-only" htmlFor="sub-q">
          Search by email address
        </label>
        <input id="sub-q" type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search by email address" maxLength={100} autoComplete="off" />
        <button type="submit">Search</button>
      </form>
      {note && (
        <p className="note ok" role="status">
          {note}
        </p>
      )}
      {!res && <p className="an-note">Loading…</p>}
      {res && !res.ok && <Problem res={res} />}
      {list && (
        <>
          <p className="an-note">
            {list.total} {list.total === 1 ? "subscriber" : "subscribers"}
            {query && ` matching “${query}”`}
          </p>
          <div className="an-scroll">
            <table className="an-table">
              <thead>
                <tr>
                  <th>Address</th>
                  <th>Status</th>
                  <th>Length</th>
                  <th>Topics</th>
                  <th>Source</th>
                  <th>Joined</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {list.subscribers.length === 0 && (
                  <tr>
                    <td colSpan={7}>Nobody found.</td>
                  </tr>
                )}
                {list.subscribers.map((s) => (
                  <tr key={s.id}>
                    <td>{s.email}</td>
                    <td>{s.status}</td>
                    <td>{s.mode}</td>
                    <td>{s.topics.join(", ") || "none"}</td>
                    <td>{s.source || "direct"}</td>
                    <td>{s.created.slice(0, 10)}</td>
                    <td>
                      <button type="button" className="an-remove" onClick={() => void remove(s.id, s.email)}>
                        Remove
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="an-pager">
            <button type="button" disabled={page <= 1} onClick={() => setPage(page - 1)}>
              Previous
            </button>
            <span>
              Page {page} of {pages}
            </span>
            <button type="button" disabled={page >= pages} onClick={() => setPage(page + 1)}>
              Next
            </button>
          </div>
        </>
      )}
    </div>
  );
}
