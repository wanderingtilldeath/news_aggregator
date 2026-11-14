"use client";

import { useActionState } from "react";
import { subscribe, type FormState } from "@/app/actions";

// The editions and reading lengths a reader can pick; the values are the ones the API accepts.
const TRACKS = [
  ["upsc", "UPSC current affairs"],
  ["engineering", "Engineering"],
];
const MODES = [
  ["1min", "1-minute read: headlines and why they matter"],
  ["7min", "7-minute read: fuller summaries"],
];

export default function SubscribeForm({ id = "footer" }: { id?: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(subscribe, null);

  return (
    <section className="signup" aria-labelledby={`signup-${id}`}>
      <h2 id={`signup-${id}`}>Postpaper by email</h2>
      <p>One edition a day, then you are done. Confirm your address to start.</p>
      <form action={action}>
        <div className="choices">
          <fieldset>
            <legend>Editions</legend>
            {TRACKS.map(([value, label]) => (
              <label key={value}>
                <input type="checkbox" name="tracks" value={value} defaultChecked /> {label}
              </label>
            ))}
          </fieldset>
          <fieldset>
            <legend>Reading length</legend>
            {MODES.map(([value, label], i) => (
              <label key={value}>
                <input type="radio" name="mode" value={value} defaultChecked={i === 0} /> {label}
              </label>
            ))}
          </fieldset>
        </div>
        <div className="row-form">
          <input type="email" name="email" required maxLength={254} placeholder="you@example.com" aria-label="Email address" autoComplete="email" />
          {/* honeypot: hidden from people, filled by simple bots */}
          <input className="hp" type="text" name="website" tabIndex={-1} autoComplete="off" aria-hidden="true" />
          <button type="submit" disabled={pending}>
            {pending ? "Sending…" : "Subscribe"}
          </button>
        </div>
      </form>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </section>
  );
}
