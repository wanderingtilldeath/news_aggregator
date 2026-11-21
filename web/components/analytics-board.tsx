"use client";

import type { Analytics } from "@/lib/api";

// The subscriber target the progress bar counts towards.
const GOAL = 500;
export const WINDOWS = [7, 30, 90];

export const pct = (n: number) => `${Math.round(n * 100)}%`;
export const sum = (xs: number[]) => xs.reduce((a, b) => a + b, 0);

export function Tile({ label, value, note }: { label: string; value: string | number; note?: string }) {
  return (
    <div className="an-tile">
      <div className="an-num">{value}</div>
      <div className="an-label">{label}</div>
      {note && <div className="an-note">{note}</div>}
    </div>
  );
}

// A bar chart as plain boxes: a column per day, its height the share of the busiest day.
export function Bars<T extends { day: string }>({ days, a, b, labelA, labelB }: { days: T[]; a: (d: T) => number; b?: (d: T) => number; labelA: string; labelB?: string }) {
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

// The window buttons shared by the tabs that count per day.
export function Windows({ days, onDays }: { days: number; onDays: (w: number) => void }) {
  return (
    <div className="an-windows" role="group" aria-label="Time window">
      {WINDOWS.map((w) => (
        <button key={w} type="button" aria-pressed={w === days} onClick={() => onDays(w)}>
          {w} days
        </button>
      ))}
    </div>
  );
}

// The audience tab: who is subscribing, from where, and what they chose.
export default function AudienceTab({ data, days, onDays }: { data: Analytics; days: number; onDays: (w: number) => void }) {
  const s = data.subscribers;
  const total = s.pending + s.active + s.unsubscribed;
  const signups = sum(data.daily.map((d) => d.signups));
  const confirmed = sum(data.daily.map((d) => d.confirmed));
  const sent = sum(data.daily.map((d) => d.sent));
  const modes = Object.entries(data.modes);

  return (
    <div className="an">
      <Windows days={days} onDays={onDays} />

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
