import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { Suspense, type ReactNode } from "react";
import { LogOutButton, RememberAccount } from "@/components/account-forms";
import AccountNav from "@/components/account-nav";
import Loading from "@/components/loading";
import { getAccount } from "@/lib/account-data";

export const metadata: Metadata = { title: "Your account · Postpaper", robots: { index: false, follow: false } };

// The frame of the logged-in area: who you are, the way around it, and the way out. The session cookie makes it a
// per-visitor page, so it is never prerendered or cached; a visitor with no session sees the log-in prompt instead.
async function Shell({ children }: { children: ReactNode }) {
  await connection();
  const res = await getAccount();
  if (!res.ok) {
    return res.signedOut ? (
      <section className="acct-card acct-empty">
        <h1>You are not logged in</h1>
        <p>Log in to see your membership, your sign-in settings and your notepad.</p>
        <div className="acct-actions">
          <Link className="btn primary" href="/login">
            Log in
          </Link>
          <Link className="btn" href="/signup">
            Sign up
          </Link>
        </div>
      </section>
    ) : (
      <p className="note err">{res.message}</p>
    );
  }

  const { username, roles } = res.data;
  const admin = roles.includes("admin");

  return (
    <div className="acct-shell">
      <RememberAccount username={username} admin={admin} />
      <aside className="acct-side">
        <div className="acct-id">
          <span className="acct-avatar" aria-hidden="true">
            {username.slice(0, 1)}
          </span>
          <div>
            <b>{username}</b>
            <span className="acct-roles">
              {roles.includes("pro") && <i>Pro</i>}
              {admin && <i>Admin</i>}
              {!roles.includes("pro") && !admin && <i className="plain">Reader</i>}
            </span>
          </div>
        </div>
        <AccountNav admin={admin} />
        <LogOutButton />
      </aside>
      <div className="acct-main">{children}</div>
    </div>
  );
}

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <main className="acct" id="view">
      <Suspense fallback={<Loading />}>
        <Shell>{children}</Shell>
      </Suspense>
    </main>
  );
}
