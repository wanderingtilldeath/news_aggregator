"use client";

import { useActionState, useState } from "react";
import { deleteSubscription, type FormState } from "@/app/actions";

// The manage page's "Delete my data": two steps, so one slip does not erase an address. The link's token is the proof.
export default function DeleteSubscriber({ token }: { token: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(deleteSubscription, null);
  const [sure, setSure] = useState(false);

  if (state?.ok) {
    return (
      <p className="note ok" role="status">
        {state.message}
      </p>
    );
  }

  return (
    <form action={action} className="manage-delete">
      <input type="hidden" name="token" value={token} />
      <h2>Delete my data</h2>
      <p className="muted">Unsubscribing keeps your address on file, marked as unsubscribed. This erases it completely, with your choices and the record of what we sent you.</p>
      {sure ? (
        <div className="manage-actions">
          <button type="submit" className="danger" disabled={pending}>
            {pending ? "Deleting…" : "Yes, delete everything"}
          </button>
          <button type="button" className="link" onClick={() => setSure(false)} disabled={pending}>
            Keep it
          </button>
        </div>
      ) : (
        <button type="button" className="btn small" onClick={() => setSure(true)}>
          Delete my data
        </button>
      )}
      {state && !state.ok && (
        <p className="note err" role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
