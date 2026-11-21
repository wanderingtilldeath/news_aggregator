"use client";

import { useActionState } from "react";
import { requestManageLink, type FormState } from "@/app/actions";

// The Log in page's form: a subscriber gives their address and is mailed the link to manage their subscription.
export default function ManageLinkForm() {
  const [state, action, pending] = useActionState<FormState, FormData>(requestManageLink, null);

  return (
    <form action={action} className="login-form">
      <label className="sr-only" htmlFor="login-email">
        Your email address
      </label>
      <input id="login-email" type="email" name="email" required maxLength={254} autoComplete="email" placeholder="you@example.com" />
      <button type="submit" disabled={pending}>
        {pending ? "Sending…" : "Email me a link"}
      </button>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
