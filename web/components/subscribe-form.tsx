"use client";

import { useActionState } from "react";
import { subscribe, type FormState } from "@/app/actions";

export default function SubscribeForm() {
  const [state, action, pending] = useActionState<FormState, FormData>(subscribe, null);

  return (
    <section className="signup" aria-labelledby="signup-h">
      <h2 id="signup-h">Postpaper by email</h2>
      <p>One short email a day, then you are done. Confirm your address to start.</p>
      <form action={action}>
        <input type="email" name="email" required maxLength={254} placeholder="you@example.com" aria-label="Email address" autoComplete="email" />
        {/* honeypot: hidden from people, filled by simple bots */}
        <input className="hp" type="text" name="website" tabIndex={-1} autoComplete="off" aria-hidden="true" />
        <button type="submit" disabled={pending}>
          {pending ? "Sending…" : "Subscribe"}
        </button>
      </form>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </section>
  );
}
