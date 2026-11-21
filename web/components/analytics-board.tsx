"use client";

import { useState } from "react";
import { adminSignOut, loadAnalytics } from "@/app/actions";
import type { Analytics } from "@/lib/api";

// The subscriber target the progress bar counts towards.
const GOAL = 500;
const WINDOWS = [7, 30, 90];

const pct = (n: number) => `${Math.round(n * 100)}%`;
const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0);

function Tile({ label, value, note }: { label: string; value: string | number; note?: string }) {
  return (
    <div className="an-tile">
      <div className="an-num">{value}</div>
      <div className="an-label">{label}</div>
      {note && <div className="an-note">{note}</div>}
    </div>
  );
}

// A bar chart as plain boxes: a column per day, its height the share of the busiest day.
function Bars({ days, a, b, labelA, labelB }: { days: Analytics["daily"]; a: (d: Analytics["daily"][number]) => number; b?: (d: Analytics["daily"][number]) => number; labelA: string; labelB?: string }) {
  const max = Math.max(1, ...days.map((d) => Math.max(a(d), b ? b(d) : 0)));
  return (
    <div className="an-chart" role="img" aria-label={`${labelA} per day, ${days.length} days`}>
      {days.map((d) => (
        <div key={d.day} className="an-col" title={`${d.day}: ${a(d)} ${labelA}${b ? `, ${b(d)} ${labelB}` : ""}`}>
          <span className="an-bar a" style={{ height: `${(a(d) / max) * 100}%` }} />
          {b && <span className="an-bar b" style={{ height: `${(b(d) / max) * 100}%` }} />}
        </div>
      ))}
    </div>
  );
}

// The audience numbers of a signed-in admin. The first window comes from the server; switching the window asks
// again through a server action that reads the session cookie, so no key is ever in the page.
export default function AnalyticsBoard({ initial }: { initial: Analytics }) {
  const [days, setDays] = useState(30);
  const [data, setData] = useState<Analytics>(initial);
  const [error, setError] = useState("");

  async function show(w: number) {
    const res = await loadAnalytics(w);
    if (res.ok) {
      setDays(w);
      setData(res.data);
      setError("");
    } else {
      setError(res.signedOut ? "Your session has ended. Reload the page to sign in again." : res.message);
    }
  }

  const s = data.subscribers;
  const total = s.pending + s.active + s.unsubscribed;
  const signups = sum(data.daily.map((d) => d.signups));
  const confirmed = sum(data.daily.map((d) => d.confirmed));
  const sent = sum(data.daily.map((d) => d.sent));
  const modes = Object.entries(data.modes);

  return (
    <div className="an">
      <div className="an-bar-row">
        <div className="an-windows" role="group" aria-label="Time window">
          {WINDOWS.map((w) => (
            <button
              key={w}
              type="button"
              aria-pressed={w === days}
              onClick={() => void show(w)}
            >
              {w} days
            </button>
          ))}
        </div>
        <form action={adminSignOut}>
          <button type="submit" className="an-forget">
            Sign out
          </button>
        </form>
      </div>

      {error && (
        <p className="note err" role="status">
          {error}
        </p>
      )}

      <section aria-label="Goal">
        <div className="an-goal-text">
          <b>{s.active}</b> active subscribers of {GOAL}
        </div>
        <div className="an-goal" role="progressbar" aria-valuemin={0} aria-valuemax={GOAL} aria-valuenow={Math.min(s.active, GOAL)}>
          <span style={{ width: `${Math.min(100, (s.active / GOAL) * 100)}%` }} />
        </div>
      </section>

      <div className="an-tiles">
        <Tile label="Active" value={s.active} note="confirmed, not unsubscribed" />
        <Tile label="Waiting to confirm" value={s.pending} />
        <Tile label="Unsubscribed" value={s.unsubscribed} note={total ? `${pct(s.unsubscribed / total)} of all signups` : undefined} />
        <Tile label="Confirm rate" value={pct(s.confirm_rate)} note="of all signups, ever" />
        <Tile label={`Signups, ${days} days`} value={signups} note={`${confirmed} confirmed`} />
        <Tile label={`Emails sent, ${days} days`} value={sent} />
      </div>

      <h2>Signups and confirmations per day</h2>
      <Bars days={data.daily} a={(d) => d.signups} b={(d) => d.confirmed} labelA="signups" labelB="confirmed" />
      <p className="an-legend">
        <span className="dot a" /> signups <span className="dot b" /> confirmed
      </p>

      <h2>Emails sent per day</h2>
      <Bars days={data.daily} a={(d) => d.sent} labelA="sent" />

      <h2>Where signups came from</h2>
      <p className="an-note">Tag a link with <code>?utm_source=linkedin</code> to see it here. Untagged visits are direct.</p>
      <table className="an-table">
        <thead>
          <tr>
            <th>Source</th>
            <th>Signups</th>
            <th>Confirmed</th>
            <th>Confirm rate</th>
          </tr>
        </thead>
        <tbody>
          {data.sources.length === 0 && (
            <tr>
              <td colSpan={4}>No signups yet.</td>
            </tr>
          )}
          {data.sources.map((r) => (
            <tr key={r.source}>
              <td>{r.source}</td>
              <td>{r.signups}</td>
              <td>{r.confirmed}</td>
              <td>{pct(r.confirm_rate)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div className="an-two">
        <div>
          <h2>Reading length</h2>
          <ul className="an-list">
            {modes.length === 0 && <li>No active subscribers yet.</li>}
            {modes.map(([m, n]) => (
              <li key={m}>
                <span>{m === "7min" ? "7-minute read" : "1-minute read"}</span>
                <b>{n}</b>
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h2>Topics followed</h2>
          <ul className="an-list">
            {data.topics.length === 0 && <li>None followed yet.</li>}
            {data.topics.map((t) => (
              <li key={t.topic}>
                <span>{t.topic}</span>
                <b>{t.count}</b>
              </li>
            ))}
          </ul>
        </div>
      </div>

      <p className="an-note">
        Page visits are in Cloudflare Web Analytics (set <code>CF_ANALYTICS_TOKEN</code>). Email opens and clicks are in your mail provider&rsquo;s dashboard: Postpaper does not track them.
      </p>
    </div>
  );
}
