import type { Metadata } from "next";
import Link from "next/link";
import { CONTACT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Terms of service · Postpaper",
  description: "The rules for using Postpaper: the free edition, accounts, Pro, summaries that can be wrong, and how to reach us.",
};

// Keep this in step with what the product does (Pro plans, accounts, the summaries). Change the date when the page changes.
const UPDATED = "21 November 2026";

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Terms of service</h1>
      <p className="lead-in">
        These are the rules for using Postpaper, the website, the daily email and the posts to Slack, Discord and Telegram. By using it you agree to them. If you do not, please do not use it. Last updated {UPDATED}.
      </p>

      <h2 className="sec-title">What Postpaper is</h2>
      <p>
        Postpaper is a daily edition of news built from other publishers&rsquo; reporting. We show a headline, a short summary and a line on why it matters, and link to the original article. The text is written by an AI model from the article.{" "}
        <b>Summaries and exam notes can be wrong or out of date.</b> Check the source before you rely on a fact, a figure or an exam answer. Postpaper is not legal, financial, medical or exam advice.
      </p>

      <h2 className="sec-title">Using the site</h2>
      <ul className="plain">
        <li>Reading the edition is free and needs no account.</li>
        <li>Use it for yourself, and for your own study or work. You may link to any page and share a story with its link.</li>
        <li>Do not copy the site in bulk, scrape it, or overload it, and do not try to break into it, or into anyone&rsquo;s account.</li>
        <li>The stories belong to their publishers. Postpaper&rsquo;s own design, code and summaries are ours; please do not republish them as your own.</li>
      </ul>

      <h2 className="sec-title">Accounts</h2>
      <p>
        An account is a username and a password, or Sign in with Google. You are responsible for what happens under it. Keep your password and your ten recovery codes safe: we have no email for you, so if you lose the password, all the
        codes and any linked Google sign-in, we cannot get the account back. Do not take a name to pass yourself off as someone else. We may remove an account that is abused. You can delete yours at any time on the{" "}
        <Link href="/account">account page</Link>.
      </p>

      <h2 className="sec-title">Pro</h2>
      <p>
        Pro is a paid plan that adds a synced notepad with notes, tags and search. You pay through Razorpay for a fixed period (the price and the length are shown on the <Link href="/pro">Pro page</Link> when you buy). It does not renew by
        itself. When the period ends you keep your notes and can read and delete them, but you cannot add or edit until you renew. To claim a membership you paste the Pro link or the payment id; whoever holds those can claim it first, so keep
        them private. For a refund, or if a payment did not give you Pro, write to <a href={`mailto:${CONTACT_EMAIL}?subject=Pro%20payment`}>{CONTACT_EMAIL}</a> and we will look at it fairly.
      </p>

      <h2 className="sec-title">The daily email and team posts</h2>
      <p>
        You can unsubscribe from the email with the link in any message, and stop a Slack or Discord post with the stop link in it. We try to send every day but do not promise delivery, or that the edition will always be on time.
      </p>

      <h2 className="sec-title">No promises, and limits</h2>
      <p>
        Postpaper is provided as it is, without a guarantee that it will always be available, correct or free of errors. We may change, pause or end any part of it, with or without notice. To the extent the law allows, we are not liable for a loss
        that comes from using Postpaper or relying on it, and our total liability for anything is limited to what you paid us in the last 12 months (nothing, for the free edition). Nothing here limits a right you have by law that cannot be
        limited.
      </p>

      <h2 className="sec-title">Changes, law and contact</h2>
      <p>
        We may change these terms; the date above shows when. Using Postpaper after a change means you accept it. These terms are governed by the laws of India, and the courts of India decide any dispute. Questions, corrections and complaints:{" "}
        <a href={`mailto:${CONTACT_EMAIL}`}>{CONTACT_EMAIL}</a>. How we handle your data is on the <Link href="/privacy">privacy page</Link>. Publishers who want a site removed: <Link href="/sources#takedown">Sources and takedown</Link>.
      </p>
    </main>
  );
}
