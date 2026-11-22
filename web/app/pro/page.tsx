import type { Metadata } from "next";
import Link from "next/link";
import { safeURL } from "@/lib/safe";
import { CONTACT_EMAIL, PRO_PLANS } from "@/lib/site";

export const metadata: Metadata = {
  title: "Postpaper Pro",
  description: "The daily edition without the sponsor line, printable as a PDF every day and every week, and sent first.",
};

export default function Page() {
  const plans = PRO_PLANS.map((p) => ({ ...p, href: safeURL(p.url) })).filter((p) => p.href);

  return (
    <main className="page" id="view">
      <h1>Postpaper Pro</h1>
      <p className="lead-in">The same free daily edition, for readers who want to keep a copy and want it first.</p>

      <ul className="plain perks">
        <li>
          <b>No sponsor line</b> in your daily email.
        </li>
        <li>
          <b>A printable edition every day</b> and a printable week, linked at the end of each email. Save them as PDFs for revision.
        </li>
        <li>
          <b>Sent first.</b> When a day has more readers than we can mail at once, Pro members are always in the first batch.
        </li>
      </ul>
      <p>
        Everyone can write and edit notes in the notepad on their own device. Pro keeps that notepad in step on every device you log in on and adds tags and search. Flashcards, the quiz and your streak are free for everyone.
      </p>

      {plans.length > 0 ? (
        <p className="pro-buy">
          {plans.map((p, i) => (
            <a key={p.id} className={i === 0 ? "btn primary big" : "btn big"} href={p.href}>
              {p.price}
            </a>
          ))}
        </p>
      ) : (
        <p className="pro-buy">
          Pro is opening soon. Meanwhile the daily edition stays free: <Link href="/#subscribe">get it by email</Link>.
        </p>
      )}

      <h2 className="sec-title">How it works</h2>
      <ul className="plain">
        <li>Pay with UPI, a card or net banking through Razorpay. We never see your card details.</li>
        <li>Use the same email address you subscribe with. Your private link arrives by email within a minute of paying.</li>
        <li>One payment buys one period. There is no automatic renewal: when it ends, the emails go back to normal and you can pay again.</li>
        <li>
          If something goes wrong, write to <a href={`mailto:${CONTACT_EMAIL}?subject=Postpaper%20Pro`}>{CONTACT_EMAIL}</a> and we will sort it out.
        </li>
      </ul>

      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
