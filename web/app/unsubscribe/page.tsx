import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import ConfirmForm from "@/components/confirm-form";

// The link carries a secret token: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Unsubscribe · Postpaper", robots: { index: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This unsubscribe link is incomplete.</p>;
  return <ConfirmForm token={token} mode="unsubscribe" />;
}

export default function Page({ searchParams }: PageProps<"/unsubscribe">) {
  return (
    <main className="narrow" id="view">
      <h1>Unsubscribe</h1>
      <p>Press the button to stop receiving Postpaper by email.</p>
      <Suspense fallback={<Loading />}>
        <Body searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
