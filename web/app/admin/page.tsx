import type { Metadata } from "next";
import { connection } from "next/server";
import { Suspense } from "react";
import Link from "next/link";
import { loadAccount, loadAnalytics } from "@/app/actions";
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

  // Not in yet. The admin is an account: first log in to it, then give the admin password.
  const account = await loadAccount();
  if (!account.ok) {
    return account.signedOut ? (
      <p>
        Log in to your account first. <Link href="/login">Log in</Link>
      </p>
    ) : (
      <p className="note err">{account.message}</p>
    );
  }
  if (!account.data.roles.includes("admin")) {
    return <p>The account <b>{account.data.username}</b> is not an admin account.</p>;
  }

  return (
    <>
      <p>
        Logged in as <b>{account.data.username}</b>. Enter the admin password to open the dashboard.
      </p>
      <AdminLogin note={res.message || undefined} />
    </>
  );
}

export default function Page() {
  return (
    <main className="narrow analytics" id="view">
      <h1>Admin</h1>
      <p>Audience, content, email sends and subscribers. Only admin accounts can open this page.</p>
      <Suspense fallback={<Loading />}>
        <Body />
      </Suspense>
    </main>
  );
}
