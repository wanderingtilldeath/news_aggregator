"use client";

import { useActionState } from "react";
import { subscribeMyEmail, type FormState } from "@/app/actions";
import { FREQUENCIES } from "@/lib/frequency";

const MODES = [
  ["1min", "1-minute read", "The headlines, each with a line on why it matters."],
  ["7min", "7-minute read", "The same stories with a fuller summary of each."],
] as const;

// Subscribes the account's own confirmed email from the account area: the same choices as the home page's signup, with no
// address to type (it is the account's).
export default function JoinNewsletter({ email }: { email: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(subscribeMyEmail, null);

  return (
    <form action={action} className="manage-form">
      <fieldset>
        <legend>Reading length</legend>
        {MODES.map(([value, label, hint], i) => (
          <label key={value} className="choice">
            <input type="radio" name="mode" value={value} defaultChecked={i === 0} />
            <span>
              {label}
              <small>{hint}</small>
            </span>
          </label>
        ))}
      </fieldset>
      <fieldset>
        <legend>How often</legend>
        {FREQUENCIES.map(([value, label, hint], i) => (
          <label key={value} className="choice">
            <input type="radio" name="frequency" value={value} defaultChecked={i === 0} />
            <span>
              {label}
              <small>{hint}</small>
            </span>
          </label>
        ))}
      </fieldset>
      <label className="topics">
        Topics to follow
        <small>Optional, up to 5, separated by commas.</small>
        <input type="text" name="topics" maxLength={200} placeholder="monsoon, rbi, isro" autoComplete="off" />
      </label>
      <div className="manage-actions">
        <button type="submit" disabled={pending}>
          {pending ? "Sending…" : `Subscribe ${email}`}
        </button>
      </div>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
