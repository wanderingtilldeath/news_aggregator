import type { Metadata } from "next";
import { connection } from "next/server";
import { Suspense } from "react";
import { loadAnalytics } from "@/app/actions";
import { AdminLogin } from "@/components/admin-forms";
import AnalyticsBoard from "@/components/analytics-board";
import Loading from "@/components/loading";

// A private page: it shows the numbers only to a signed-in admin, and keeps search engines out.
export const metadata: Metadata = { title: "Analytics · Postpaper", robots: { index: false, follow: false }, referrer: "no-referrer" };

async function Body() {
  // The session cookie makes this a per-visitor page: never prerendered or cached.
  await connection();
  const res = await loadAnalytics(30);
  if (res.ok) return <AnalyticsBoard initial={res.data} />;

  return (
    <>
      <p>Sign in with an admin email address. A one-time link is sent to it.</p>
      <AdminLogin note={res.message || undefined} />
    </>
  );
}

export default function Page() {
  return (
    <main className="narrow analytics" id="view">
      <h1>Analytics</h1>
      <p>Who is subscribing, from where, and what they chose. Counts only: no address is shown.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
    </main>
  );
}
