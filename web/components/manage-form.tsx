"use client";

import Link from "next/link";
import { useActionState } from "react";
import { saveSubscription, type FormState } from "@/app/actions";
import type { SubscriptionInfo } from "@/lib/api";

const MODES = [
  ["1min", "1-minute read", "The headlines, each with a line on why it matters."],
  ["7min", "7-minute read", "The same stories with a fuller summary of each."],
];

// The manage page's form: the reading length and the words to follow. Saving needs only the link's token,
// the same one the unsubscribe link carries.
export default function ManageForm({ token, sub }: { token: string; sub: SubscriptionInfo }) {
  const [state, action, pending] = useActionState<FormState, FormData>(saveSubscription, null);

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
