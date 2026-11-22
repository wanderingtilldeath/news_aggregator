import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { VerifyEmailButton } from "@/components/account-forms";

// The link carries a secret token: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Confirm your email · Postpaper", robots: { index: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This link is incomplete. Ask for a new one on your account page.</p>;
  return <VerifyEmailButton token={token} />;
}

export default function Page({ searchParams }: PageProps<"/verify-email">) {
  return (
    <main className="narrow" id="view">
      <h1>Confirm your email</h1>
      <p>Press the button to confirm this address for your Postpaper account.</p>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
