"use client";

import { useActionState } from "react";
import { confirm, stopTeam, unsubscribe, type FormState } from "@/app/actions";

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

  return (
    <>
      {!state?.ok && (
        <form action={action}>
          <input type="hidden" name="token" value={token} />
          <button type="submit" disabled={pending}>
            {pending ? m.busy : m.label}
          </button>
        </form>
      )}
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </>
  );
}
