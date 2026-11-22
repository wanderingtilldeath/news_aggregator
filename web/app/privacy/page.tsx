import type { Metadata } from "next";
import Link from "next/link";
import { CONTACT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Privacy · Postpaper",
  description: "What Postpaper keeps about readers, subscribers, accounts and Pro members, who else handles it, and how to have it removed.",
};

// Keep this page in step with what the code stores: the subscribers, accounts, pro_members and team_hooks tables, the
// cookies and browser keys, and the services named below. Change the date when the page changes.
const UPDATED = "22 November 2026";

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Privacy</h1>
      <p className="lead-in">
        Postpaper is a daily news edition. You can read all of it without an account, an email address or a cookie banner. This page says what we keep when you do sign up for something, and who handles it for us. Last updated {UPDATED}.
      </p>

      <h2 className="sec-title">If you only read</h2>
      <ul className="plain">
        <li>We do not ask who you are and we set no tracking cookies.</li>
        <li>Page counts come from Cloudflare Web Analytics, which does not use cookies and does not follow you from site to site. It is loaded only when the site owner has switched it on.</li>
        <li>
          Your city and its temperature in the page header are worked out from your IP address by our host (Vercel) while the page loads. We do not store the city or the address. The weather comes from Open-Meteo, which is asked for a
          coordinate rounded to a city, never for your address.
        </li>
        <li>
          Your saved stories, notepad, quiz progress, flashcards, reading streak, theme and reading settings stay in your own browser (local storage). We cannot see them. Clearing your browser data removes them, unless you have an
          account: then your saved stories, flashcard progress, streak and missed quiz questions are also kept on our server so they follow you to your other devices, and a Pro member&rsquo;s notepad is kept there too (below). Logging out
          clears them from that browser, and deleting the account deletes the copy on our server.
        </li>
      </ul>

      <h2 className="sec-title">Email subscribers</h2>
      <p>
        To send the daily email we keep your email address, the length and topics you chose, a secret token for your manage and unsubscribe links, and the dates you signed up, confirmed and unsubscribed. We also record which mail went to whom on
        which day, so no one gets the same edition twice. We use the address only for the edition and for the confirmation and manage-link mails. We do not sell it or share it with advertisers. The email is sent through Brevo.
      </p>
      <p>
        Unsubscribing stops the mail but keeps the address, marked as unsubscribed, so we do not mail you again by mistake. To have it all deleted, open the link at the bottom of any of our emails and press &ldquo;Delete my data&rdquo; on the
        manage page: the address, your choices and the delivery records go at once. If you cannot reach that link, write to <a href={`mailto:${CONTACT_EMAIL}?subject=Delete%20my%20data`}>{CONTACT_EMAIL}</a> from the address. An address that signs up
        and never confirms is deleted automatically after 30 days.
      </p>

      <h2 className="sec-title">Accounts</h2>
      <p>
        An account needs a username and a password. An email address is optional: if you add one, we keep it so you can log in with it and reset a forgotten password. We mail it a link to confirm it, and a reset link when you ask for one, and nothing else. Until it is confirmed it cannot be used to log in. We keep the username, a salted hash of the password (never the password), hashes of your ten recovery codes, hashes of your logged-in sessions, and, if you link
        Google, Google&rsquo;s identifier for you. We do not receive or keep your Google email address or name. A cookie keeps you logged in; it is needed for that and for nothing else.
      </p>
      <p>
        If you added no confirmed email, we cannot reset a password for you: use one of your recovery codes. The links we mail work once and expire (24 hours to confirm an address, one hour to reset a password). You can delete your account yourself on the <Link href="/account">account page</Link>, which removes the username, the optional email, hashes, sessions and the Google link at once.
      </p>

      <h2 id="google" className="sec-title">Sign in with Google</h2>
      <p>
        Sign in with Google is optional. If you use it, Postpaper asks Google for one thing only: the OpenID <code>openid</code> scope, which tells us a stable, anonymous identifier for your Google account. We do not ask for, receive or keep your
        name, email address, profile picture, contacts, calendar, files or any other Google data.
      </p>
      <ul className="plain">
        <li>
          <b>What we access:</b> Google&rsquo;s identifier for your account (the &ldquo;sub&rdquo; value of the sign-in token), checked at the moment you sign in.
        </li>
        <li>
          <b>How we use it:</b> only to recognise you the next time you sign in with Google, and to let you link Google to an account as a way back in. Nothing else.
        </li>
        <li>
          <b>Where it is stored:</b> in our database, next to your Postpaper account, until you delete the account or ask us to remove it.
        </li>
        <li>
          <b>Who it is shared with:</b> no one. We do not sell it, share it, use it for advertising, or use it to build profiles.
        </li>
        <li>
          <b>How to remove it:</b> delete your account on the <Link href="/account">account page</Link>, or write to <a href={`mailto:${CONTACT_EMAIL}?subject=Remove%20Google%20link`}>{CONTACT_EMAIL}</a>. You can also revoke Postpaper&rsquo;s access
          in your Google account settings under Security, Third-party apps and services.
        </li>
      </ul>
      <p>Postpaper&rsquo;s use of information received from Google APIs follows the Google API Services User Data Policy, including the Limited Use requirements.</p>

      <h2 className="sec-title">Pro members</h2>
      <p>
        When you pay for Pro through Razorpay we receive the payment id, the amount and the email Razorpay has for you, and keep them to run your membership. We never see your card or UPI details; Razorpay holds those. The Pro notepad, if you
        use it, is kept on our server under your membership: the sentences you saved, your notes and tags. You can delete them from the notepad at any time, and we delete them if you ask.
      </p>

      <h2 className="sec-title">Slack and Discord channels</h2>
      <p>
        If you connect a channel, we keep its webhook address (a secret), the edition you chose, a token for the stop link and a count of recent refused posts. The address is not shown again on any page, including the admin dashboard; it is
        readable only by the operator of the database, and it is used for nothing except posting the edition to that channel. The stop link in each post deletes all of it, and so does a channel that keeps refusing our posts (three days
        in a row) or whose webhook was deleted.
      </p>

      <h2 className="sec-title">Telegram</h2>
      <p>
        The Postpaper channels on Telegram carry the same public edition as the site, and the posts hold no reader data. If you message the bot (for example <code>/today</code> or <code>/quiz</code>), Telegram passes your message and chat
        number to our bot only to answer it. We do not store them, keep a list of bot users, or message you unprompted. Telegram keeps its own records of your chats under its own policy.
      </p>

      <h2 className="sec-title">Who else handles data</h2>
      <ul className="plain">
        <li>Vercel hosts this website and Render hosts the API. Cloudflare serves the front pages and, if switched on, counts visits.</li>
        <li>Neon hosts the database.</li>
        <li>Brevo sends the email. Razorpay takes payments. Google handles Sign in with Google, if you use it.</li>
        <li>An AI model (Gemini) writes the summaries from public news articles. It is never sent anything about readers.</li>
      </ul>
      <p>These services may keep their own logs, such as IP addresses, under their own policies. We do not sell data, show ads or build profiles of readers.</p>

      <h2 className="sec-title">Your choices</h2>
      <p>
        Write to <a href={`mailto:${CONTACT_EMAIL}?subject=My%20data`}>{CONTACT_EMAIL}</a> to see, correct or delete what we hold about you. Write from the email address in question so we can tell it is yours. We reply within a few days. For
        publishers who want a site removed from the edition, see <Link href="/sources#takedown">Sources and takedown</Link>. The rules for using Postpaper are in the <Link href="/terms">terms of service</Link>.
      </p>
    </main>
  );
}
