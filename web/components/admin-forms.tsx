"use client";

import { useActionState } from "react";
import { adminSignIn, type FormState } from "@/app/actions";

// The admin sign-in: an address from ADMIN_EMAILS and the shared password. The password goes to the server action,
// which sends it to the API; the session that comes back is kept in an httpOnly cookie.
export function AdminLogin({ note }: { note?: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(adminSignIn, null);
  const message = state?.message ?? note;

  return (
    <form action={action} className="an-key">
      <label>
        Admin email
        <input type="email" name="email" required maxLength={254} autoComplete="username" placeholder="you@example.com" />
      </label>
      <label>
        Password
        <input type="password" name="password" required maxLength={200} autoComplete="current-password" />
      </label>
      <button type="submit" disabled={pending}>
        {pending ? "Signing in…" : "Sign in"}
      </button>
      {message && (
        <p className="note err" role="status">
          {message}
        </p>
      )}
    </form>
  );
}
