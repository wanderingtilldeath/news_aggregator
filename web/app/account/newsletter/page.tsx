import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { loadAccountNewsletter } from "@/app/actions";
import { Card } from "@/components/account-ui";
import DeleteSubscriber from "@/components/delete-subscriber";
import JoinNewsletter from "@/components/join-newsletter";
import ManageForm from "@/components/manage-form";
import { getSubscription } from "@/lib/api";

export const metadata: Metadata = { title: "Newsletter · Postpaper", robots: { index: false, follow: false } };

// The daily email, managed from the account. Which subscriber it is comes from the account's CONFIRMED email, so no
// mailed link is needed: the same choices as the manage page (length, rhythm, topics, unsubscribe, delete), plus
// signing up again.
export default async function Page() {
  await connection();
  const res = await loadAccountNewsletter();
  if (!res.ok) return <p className="note err">{res.message}</p>;
  const { email, email_confirmed, subscribed, token } = res.data;

  const sub = token ? await getSubscription(token) : null;
  if (sub === "error") return <p className="note err">We could not load your newsletter just now. Please try again in a little while.</p>;

  return (
    <>
      <h1>Newsletter</h1>

      {!email_confirmed && (
        <Card title="Confirm an email first" hint="The daily email goes to an address, so this page needs one on your account.">
          <p className="acct-text">
            Add an email on <Link href="/account/security">Sign-in and security</Link> and open the link we send to confirm it. Then your newsletter settings appear here. Until then you can still use the <b>Manage your subscription</b> link at the
            bottom of any Postpaper email, or <Link href="/login">ask for one</Link> on the log-in page.
          </p>
          <div className="acct-actions">
            <Link className="btn primary" href="/account/security">
              Add or confirm an email
            </Link>
          </div>
        </Card>
      )}

      {email_confirmed && (!sub || sub.status === "unsubscribed") && (
        <Card
          title={sub ? "You are unsubscribed" : "You are not subscribed"}
          hint={sub ? `The daily email to ${email} is switched off. Subscribe again any time.` : `Get the daily briefing at ${email}. It is free, and we mail a link to confirm first.`}
        >
          <JoinNewsletter email={email ?? ""} />
        </Card>
      )}

      {sub && sub.status !== "unsubscribed" && token && (
        <>
          <Card title="Your daily email" hint={sub.status === "pending" ? `Not confirmed yet: open the link we mailed to ${email} to start receiving it.` : `Sent to ${email}.`}>
            <ManageForm token={token} sub={sub} />
          </Card>
        </>
      )}

      {/* Erases the address from the newsletter, with the choices and the record of what was sent; the account stays. */}
      {subscribed && token && <DeleteSubscriber token={token} />}
    </>
  );
}
