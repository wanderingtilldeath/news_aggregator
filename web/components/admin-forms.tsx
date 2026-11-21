"use client";

import { useActionState } from "react";
import { adminSignIn, type FormState } from "@/app/actions";

// The admin step-up: the shared admin password, entered after logging in to an account that is on the admin list. The
// password goes to the server action, which sends it with the account's session to the API; the admin session that
// comes back is kept in an httpOnly cookie.
export function AdminLogin({ note }: { note?: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(adminSignIn, null);
  const message = state?.message ?? note;

  return (
    <form action={action} className="an-key">
      <label>
        Admin password
        <input type="password" name="password" required maxLength={200} autoComplete="off" />
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
