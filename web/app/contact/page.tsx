import type { Metadata } from "next";
import Link from "next/link";
import { CONTACT_EMAIL } from "@/lib/site";

export const metadata: Metadata = {
  title: "Contact and support · Postpaper",
  description: "How to reach Postpaper for help with the daily email, your account, Pro, a correction or a takedown.",
};

const mail = (subject: string) => `mailto:${CONTACT_EMAIL}?subject=${encodeURIComponent(subject)}`;

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Contact and support</h1>
      <p className="lead-in">
        Write to <a href={mail("Postpaper support")}>{CONTACT_EMAIL}</a>. We reply within a few days. Write from the address you signed up with, so we can find your account.
      </p>

      <h2 className="sec-title">What to write about</h2>
      <ul className="plain">
        <li>
          <b>The daily email:</b> not arriving, wrong topics, or daily versus weekly. Check spam first, or change it on the link at the foot of any edition.
        </li>
        <li>
          <b>Your account or Pro:</b> <a href={mail("Postpaper Pro")}>write to us</a> about login, billing or removing your data.
        </li>
        <li>
          <b>A wrong summary or a correction:</b> <a href={mail("Postpaper correction")}>send the story link</a> and what is wrong.
        </li>
        <li>
          <b>A publisher takedown:</b> see <Link href="/sources">Sources and takedown</Link>.
        </li>
      </ul>
    </main>
  );
}
