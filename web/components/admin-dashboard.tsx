"use client";

import { useState } from "react";
import { adminSignOut, loadAnalytics } from "@/app/actions";
import type { Analytics } from "@/lib/api";
import AudienceTab from "./analytics-board";
import { ContentTab, EmailsTab, ProTab, SubscribersTab } from "./admin-tabs";

const TABS = [
  ["audience", "Audience"],
  ["content", "Content"],
  ["emails", "Emails"],
  ["subscribers", "Subscribers"],
  ["pro", "Pro"],
] as const;

type Tab = (typeof TABS)[number][0];

// The admin dashboard: five tabs over one signed-in session. The audience numbers come from the server first;
// every later read goes through a server action that reads the session cookie, so no secret is in the page.
export default function AdminDashboard({ initial }: { initial: Analytics }) {
  const [tab, setTab] = useState<Tab>("audience");
  const [days, setDays] = useState(30);
  const [audience, setAudience] = useState<Analytics>(initial);
  const [error, setError] = useState("");

  async function show(w: number) {
    const res = await loadAnalytics(w);
    if (res.ok) {
      setDays(w);
      setAudience(res.data);
      setError("");
    } else {
      setError(res.signedOut ? "Your session has ended. Reload the page to sign in again." : res.message);
    }
  }

  return (
    <div className="ad">
      <div className="ad-head">
        <div className="ad-tabs" role="tablist" aria-label="Dashboard">
          {TABS.map(([id, label]) => (
            <button key={id} type="button" role="tab" id={`tab-${id}`} aria-selected={tab === id} aria-controls={`panel-${id}`} onClick={() => setTab(id)}>
              {label}
            </button>
          ))}
        </div>
        <form action={adminSignOut}>
          <button type="submit" className="an-forget">
            Sign out
          </button>
        </form>
      </div>

      {error && (
        <p className="note err" role="status">
          {error}
        </p>
      )}

      <div role="tabpanel" id={`panel-${tab}`} aria-labelledby={`tab-${tab}`}>
        {tab === "audience" && <AudienceTab data={audience} days={days} onDays={(w) => void show(w)} />}
        {tab === "content" && <ContentTab />}
        {tab === "emails" && <EmailsTab data={audience} days={days} onDays={(w) => void show(w)} />}
        {tab === "subscribers" && <SubscribersTab />}
        {tab === "pro" && <ProTab />}
      </div>
    </div>
  );
}
