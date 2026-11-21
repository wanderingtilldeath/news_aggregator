import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { AdminVerify } from "@/components/admin-forms";

// The link carries a one-time secret: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Admin sign-in · Postpaper", robots: { index: false, follow: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This sign-in link is incomplete. Open it from the email.</p>;
  return <AdminVerify token={token} />;
}

export default function Page({ searchParams }: PageProps<"/admin/verify">) {
  return (
    <main className="narrow" id="view">
      <h1>Admin sign-in</h1>
      <p>Press the button to sign in. This link works once.</p>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
