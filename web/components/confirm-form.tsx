"use client";

import { useRouter } from "next/navigation";
import { useActionState, useEffect } from "react";
import { confirm, stopTeam, unsubscribe, type FormState } from "@/app/actions";
import SlowNote from "./slow-note";

const MODES = {
  confirm: { action: confirm, label: "Confirm my email address", busy: "Confirming…" },
  unsubscribe: { action: unsubscribe, label: "Unsubscribe", busy: "Unsubscribing…" },
  stopTeam: { action: stopTeam, label: "Stop the posts", busy: "Stopping…" },
};

// Confirming and unsubscribing are button presses, not GETs, so mail scanners that
// open every link in an email cannot do either for the reader.
export default function ConfirmForm({ token, mode = "confirm" }: { token: string; mode?: keyof typeof MODES }) {
  const m = MODES[mode];
  const [state, action, pending] = useActionState<FormState, FormData>(m.action, null);
  const router = useRouter();

  // Once the address is confirmed, leave a moment to read the message, then go to the home page.
  useEffect(() => {
    if (mode !== "confirm" || !state?.ok) return;
    const t = setTimeout(() => router.push("/"), 2000);
    return () => clearTimeout(t);
  }, [mode, state, router]);

  return (
    <>
      {!state?.ok && (
        <form action={action}>
          <input type="hidden" name="token" value={token} />
          <button type="submit" disabled={pending} aria-busy={pending}>
            {pending ? m.busy : m.label}
          </button>
        </form>
      )}
      {pending && <SlowNote />}
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
          {mode === "confirm" && state.ok && " Taking you to the home page…"}
        </p>
      )}
    </>
  );
}
