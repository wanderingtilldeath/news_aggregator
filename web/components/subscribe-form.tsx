"use client";

import { useActionState } from "react";
import { subscribe, type FormState } from "@/app/actions";

// The reading lengths a reader can pick; the values are the ones the API accepts. Every reader gets the same
// top-news edition, so there is no edition to pick; the length defaults to the one-minute read.
const MODES = [
  ["1min", "1-minute read"],
  ["7min", "7-minute read"],
];

export default function SubscribeForm({ id = "footer", compact = false }: { id?: string; compact?: boolean }) {
  const [state, action, pending] = useActionState<FormState, FormData>(subscribe, null);

  return (
    <section className={compact ? "signup compact" : "signup"} aria-labelledby={`signup-${id}`}>
      <h2 id={`signup-${id}`}>{compact ? "Subscribe to our newsletter" : "Postpaper by email"}</h2>
      <p>{compact ? "One edition a day, free, in your inbox." : "Free. Confirm your address and your first briefing arrives tomorrow morning."}</p>
      <form action={action}>
        <div className="row-form">
          <input type="email" name="email" required maxLength={254} placeholder="you@example.com" aria-label="Email address" autoComplete="email" />
          {/* honeypot: hidden from people, filled by simple bots */}
          <input className="hp" type="text" name="website" tabIndex={-1} autoComplete="off" aria-hidden="true" />
          <button type="submit" disabled={pending}>
            {pending ? "Sending…" : "Subscribe"}
          </button>
        </div>
        <div className="choices">
          <fieldset>
            <legend>Reading length</legend>
            {MODES.map(([value, label], i) => (
              <label key={value}>
                <input type="radio" name="mode" value={value} defaultChecked={i === 0} /> {label}
              </label>
            ))}
          </fieldset>
          <label className="topics">
            Topics to follow (optional, up to 5, separated by commas)
            <input type="text" name="topics" maxLength={200} placeholder="monsoon, rbi, isro" autoComplete="off" />
          </label>
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
