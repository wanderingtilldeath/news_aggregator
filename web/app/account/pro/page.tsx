import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { Card } from "@/components/account-ui";
import { LinkProForm } from "@/components/account-forms";
import { getAccount } from "@/lib/account-data";
import { safeURL } from "@/lib/safe";
import { PRO_PLANS } from "@/lib/site";

export const metadata: Metadata = { title: "Membership · Postpaper", robots: { index: false, follow: false } };

export default async function Page() {
  await connection();
  const res = await getAccount();
  if (!res.ok) return null;
  const { pro } = res.data;
  // The payment links of the plans on offer (a plan with no link is not offered).
  const plans = PRO_PLANS.map((p) => ({ ...p, href: safeURL(p.url) })).filter((p) => p.href);
  // Renewing early adds to the days left, so the buttons are always there for a member, and louder in the last week.
  const renewSoon = pro.active && pro.days_left <= 7;

  return (
    <>
      <h1>Membership</h1>
      <p className="acct-lead">Postpaper Pro adds a notepad that syncs on every device you log in on.</p>

      {pro.active && (
        <Card title="Pro is active" hint={`Until ${pro.until}`}>
          <p className="acct-text">
            <b>{pro.days_left === 1 ? "1 day" : `${pro.days_left} days`} left.</b> Renewing adds to the days you have left, so renewing early loses nothing.
          </p>
          <p className="acct-text">Your notepad syncs on every device you log in on, and you can tag notes and search across everything you saved.</p>
          {pro.email && (
            <p className="acct-text">
              Pro is applied to <b>{pro.email}</b>, the address you paid with. Your daily email has no sponsor line only when you subscribe with that address.
            </p>
          )}
          <div className="acct-actions">
            <Link className="btn primary" href="/notebook">
              Open your notepad
            </Link>
            {pro.edition && (
              <Link className="btn" href={pro.edition}>
                Printable edition
              </Link>
            )}
          </div>
        </Card>
      )}

      {pro.linked && !pro.active && (
        <Card title="Your Pro membership ended" hint={`Ended on ${pro.until}`}>
          <p className="acct-text">Your notes are still yours: you can read, remove and export them. Renew to sync, edit and add notes again.</p>
        </Card>
      )}

      {pro.linked && (
        <Card
          title={pro.active ? "Renew or extend" : "Renew Pro"}
          hint={pro.active ? (renewSoon ? `Pro ends in ${pro.days_left === 1 ? "1 day" : `${pro.days_left} days`}.` : "Renewing early adds to the days you have left.") : "Pay again to switch Pro back on."}
        >
          <p className="acct-text">There is no automatic renewal: Pro simply ends on the date above unless you pay again, so there is nothing to cancel. Pay with the same email address ({pro.email}) so it extends this membership.</p>
          <div className="acct-actions">
            {plans.length > 0 ? (
              plans.map((p, i) => (
                <a key={p.id} className={i === 0 && (renewSoon || !pro.active) ? "btn primary" : "btn"} href={p.href}>
                  {p.price}
                </a>
              ))
            ) : (
              <Link className="btn primary" href="/pro">
                See Pro plans
              </Link>
            )}
          </div>
        </Card>
      )}

      {pro.linked && pro.payments && pro.payments.length > 0 && (
        <Card title="Payments" hint="Each payment on record for this membership. The reference is the payment id on your Razorpay receipt, which is what to quote if you write to us.">
          <table className="acct-table">
            <thead>
              <tr>
                <th>Date</th>
                <th>Reference</th>
              </tr>
            </thead>
            <tbody>
              {pro.payments.map((p) => (
                <tr key={p.id}>
                  <td>{p.date}</td>
                  <td>
                    <code>{p.id}</code>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
          <li>Tags and search across your notes. (Writing and editing notes is free for everyone.)</li>
          <li>Printable daily and weekly editions.</li>
          <li>No sponsor line in the daily email.</li>
        </ul>
      </Card>
    </>
  );
}
