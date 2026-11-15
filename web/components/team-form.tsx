"use client";

import { useActionState } from "react";
import { joinTeam, type FormState } from "@/app/actions";

const TRACKS = [
  ["engineering", "Engineering"],
  ["upsc", "UPSC current affairs"],
];

// Connect a channel: paste its incoming webhook address and choose an edition. The address is a
// secret, so the field hides it, and the server posts a test message before it keeps anything.
export default function TeamForm() {
  const [state, action, pending] = useActionState<FormState, FormData>(joinTeam, null);

  return (
    <form action={action} className="team-form">
      <label>
        Webhook address
        <input type="password" name="url" required maxLength={300} placeholder="https://hooks.slack.com/services/…  or  https://discord.com/api/webhooks/…" autoComplete="off" spellCheck={false} />
      </label>
      <fieldset>
        <legend>Edition</legend>
        {TRACKS.map(([value, label], i) => (
          <label key={value}>
            <input type="radio" name="track" value={value} defaultChecked={i === 0} /> {label}
          </label>
        ))}
      </fieldset>
      <button type="submit" className="btn primary" disabled={pending}>
        {pending ? "Connecting…" : "Connect the channel"}
      </button>
      {state && (
        <p className={state.ok ? "note ok" : "note err"} role="status">
          {state.message}
        </p>
      )}
    </form>
  );
}
