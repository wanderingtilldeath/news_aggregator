import type { Metadata } from "next";
import Link from "next/link";
import { connection } from "next/server";
import { Suspense } from "react";
import Loading from "@/components/loading";
import DeleteSubscriber from "@/components/delete-subscriber";
import ManageForm from "@/components/manage-form";
import { asFrequency } from "@/lib/frequency";
import { getSubscription } from "@/lib/api";

// The link carries a secret token: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Manage your subscription · Postpaper", robots: { index: false, follow: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  // A reader's own record: never prerendered or cached.
  await connection();
  const { token, frequency } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This link is incomplete. Open it from the link at the bottom of a Postpaper email.</p>;

  const sub = await getSubscription(token);
  if (sub === "error") return <p className="note err">We could not load your subscription just now. Please try again in a little while.</p>;
  if (!sub) return <p className="note err">This link is not valid. Open it from the link at the bottom of a recent Postpaper email.</p>;

  if (sub.status === "unsubscribed") {
    return (
      <>
        <p>
          {sub.email} is unsubscribed, so no emails are sent. <Link href="/#subscribe">Sign up again</Link> any time to get the briefing.
        </p>
        <DeleteSubscriber token={token} />
      </>
    );
  }

  return (
    <>
      <p className="muted">
        For <b>{sub.email}</b>
        {sub.status === "pending" && ". You have not confirmed this address yet: open the confirmation email we sent to start receiving the briefing"}
        {"."}
      </p>
      <ManageForm token={token} sub={sub} suggested={asFrequency(frequency)} />
      <DeleteSubscriber token={token} />
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/manage">) {
  return (
    <main className="narrow manage" id="view">
      <h1>Manage your subscription</h1>
      <p>Choose how often the edition comes, how long it is, and which topics get their own section.</p>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
