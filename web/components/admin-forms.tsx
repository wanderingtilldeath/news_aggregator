"use client";

import { useActionState } from "react";
import { adminSignIn, requestAdminLink, type FormState } from "@/app/actions";

// Step one: an admin gives their address and is mailed a one-time link.
export function AdminLogin({ note }: { note?: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(requestAdminLink, null);

  return (
    <form action={action} className="an-key">
      <label>
        Admin email
        <input type="email" name="email" required maxLength={254} autoComplete="email" placeholder="you@example.com" />
      </label>
      <button type="submit" disabled={pending}>
        {pending ? "Sending…" : "Email me a sign-in link"}
      </button>
      {(state ?? (note ? { ok: false, message: note } : null)) && (
        <p className={(state ?? { ok: false }).ok ? "note ok" : "note err"} role="status">
          {state?.message ?? note}
        </p>
      )}
    </form>
  );
}

// Step two: pressing the button on the link's page uses the link up. A button, not the link itself, so a mail
// scanner that opens every link in an email cannot use it for the admin.
export function AdminVerify({ token }: { token: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(adminSignIn, null);

  return (
    <form action={action}>
      <input type="hidden" name="token" value={token} />
      <button type="submit" disabled={pending}>
        {pending ? "Signing in…" : "Sign in"}
      </button>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
