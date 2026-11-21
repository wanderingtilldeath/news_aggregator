"use client";

import Link from "next/link";
import { useActionState } from "react";
import { saveSubscription, type FormState } from "@/app/actions";
import type { SubscriptionInfo } from "@/lib/api";
import { FREQUENCIES, type Frequency } from "@/lib/frequency";

const MODES = [
  ["1min", "1-minute read", "The headlines, each with a line on why it matters."],
  ["7min", "7-minute read", "The same stories with a fuller summary of each."],
];

// The manage page's form: the reading length, how often the edition comes, and the words to follow. Saving needs only
// the link's token, the same one the unsubscribe link carries. suggested is the rhythm the email's "get it weekly
// instead" link asked for: it is picked here, and nothing changes until the reader saves.
export default function ManageForm({ token, sub, suggested }: { token: string; sub: SubscriptionInfo; suggested?: Frequency }) {
  const [state, action, pending] = useActionState<FormState, FormData>(saveSubscription, null);
  const chosen = suggested ?? sub.frequency;

  return (
    <form action={action} className="manage-form">
      <input type="hidden" name="token" value={token} />
      <fieldset>
        <legend>Reading length</legend>
        {MODES.map(([value, label, hint]) => (
          <label key={value} className="choice">
            <input type="radio" name="mode" value={value} defaultChecked={sub.mode === value} />
            <span>
              {label}
              <small>{hint}</small>
            </span>
          </label>
        ))}
      </fieldset>
      <fieldset>
        <legend>How often</legend>
        {FREQUENCIES.map(([value, label, hint]) => (
          <label key={value} className="choice">
            <input type="radio" name="frequency" value={value} defaultChecked={chosen === value} />
            <span>
              {label}
              <small>{hint}</small>
            </span>
          </label>
        ))}
        {suggested && suggested !== sub.frequency && <small className="note">Press Save changes to switch to {suggested === "weekly" ? "the weekly" : "the daily"} email.</small>}
      </fieldset>
      <label className="topics">
        Topics to follow
        <small>Optional, up to 5, separated by commas. Stories that mention them get their own section.</small>
        <input type="text" name="topics" maxLength={200} defaultValue={sub.topics.join(", ")} placeholder="monsoon, rbi, isro" autoComplete="off" />
      </label>
      <div className="manage-actions">
        <button type="submit" disabled={pending}>
          {pending ? "Saving…" : "Save changes"}
        </button>
        <Link href={`/unsubscribe?token=${encodeURIComponent(token)}`}>Unsubscribe instead</Link>
      </div>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
