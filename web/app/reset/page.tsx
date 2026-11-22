import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { ResetForm } from "@/components/account-forms";

// The link carries a secret token: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Choose a new password · Postpaper", robots: { index: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This link is incomplete. Ask for a new one on the <Link href="/forgot">forgot password page</Link>.</p>;
  return <ResetForm token={token} />;
}

export default function Page({ searchParams }: PageProps<"/reset">) {
  return (
    <main className="narrow login" id="view">
      <h1>Choose a new password</h1>
      <p>At least ten characters. Every other device is logged out when you save it.</p>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
