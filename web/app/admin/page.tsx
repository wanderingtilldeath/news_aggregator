import type { Metadata } from "next";
import { connection } from "next/server";
import { Suspense } from "react";
import { loadAnalytics } from "@/app/actions";
import AdminDashboard from "@/components/admin-dashboard";
import { AdminLogin } from "@/components/admin-forms";
import Loading from "@/components/loading";

// A private page: it shows the dashboard only to a signed-in admin, and keeps search engines out.
export const metadata: Metadata = { title: "Admin · Postpaper", robots: { index: false, follow: false }, referrer: "no-referrer" };

async function Body() {
  // The session cookie makes this a per-visitor page: never prerendered or cached.
  await connection();
  const res = await loadAnalytics(30);
  if (res.ok) return <AdminDashboard initial={res.data} />;

  return (
    <>
      <p>Sign in with an admin email address and the admin password.</p>
      <AdminLogin note={res.message || undefined} />
    </>
  );
}

export default function Page() {
  return (
    <main className="narrow analytics" id="view">
      <h1>Admin</h1>
      <p>Audience, content, email sends and subscribers. Only people on the admin list can see this page.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
    </main>
  );
}
