import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { Card } from "@/components/account-ui";
import { LinkProForm } from "@/components/account-forms";
import { getAccount } from "@/lib/account-data";

export const metadata: Metadata = { title: "Membership · Postpaper", robots: { index: false, follow: false } };

export default async function Page() {
  await connection();
  const res = await getAccount();
  if (!res.ok) return null;
  const { pro } = res.data;

  return (
    <>
      <h1>Membership</h1>
      <p className="acct-lead">Postpaper Pro adds a notepad that syncs on every device you log in on.</p>

      {pro.active && (
        <Card title="Pro is active" hint={`Until ${pro.until}`}>
          <p className="acct-text">Your notepad syncs on every device you log in on, and you can add notes, tags and search across everything you saved.</p>
          <div className="acct-actions">
            <Link className="btn primary" href="/notebook">
              Open your notepad
            </Link>
            <Link className="btn" href="/pro">
              Plans and renewal
            </Link>
          </div>
        </Card>
      )}

      {pro.linked && !pro.active && (
        <Card title="Your Pro membership ended" hint={`Ended on ${pro.until}`}>
          <p className="acct-text">Your notes are still yours: you can read, delete and export them. Renew to sync and add notes again.</p>
          <div className="acct-actions">
            <Link className="btn primary" href="/pro">
              Renew Pro
            </Link>
          </div>
        </Card>
      )}

      {!pro.linked && (
        <>
          <Card title="Link your membership" hint="Already paid for Pro? Tie it to this account once.">
            <LinkProForm />
          </Card>
          <Card title="Not a member yet?">
            <p className="acct-text">Reading Postpaper is free. Pro is optional.</p>
            <div className="acct-actions">
              <Link className="btn primary" href="/pro">
                See Pro plans
              </Link>
            </div>
          </Card>
        </>
      )}

      <Card title="What Pro includes">
        <ul className="acct-list">
          <li>A notepad that syncs across your devices, with up to 10,000 notes.</li>
          <li>Your own note under each saved sentence, with tags and search.</li>
          <li>Printable daily and weekly editions.</li>
          <li>No sponsor line in the daily email.</li>
        </ul>
      </Card>
    </>
  );
}
