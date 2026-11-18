import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import ConfirmForm from "@/components/confirm-form";

// The link carries a secret token: keep it out of search indexes and Referer headers.
export const metadata: Metadata = { title: "Confirm your email · Postpaper", robots: { index: false }, referrer: "no-referrer" };

async function Body({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token !== "string" || !token) return <p className="note err">This confirmation link is incomplete.</p>;
  return <ConfirmForm token={token} />;
}

export default function Page({ searchParams }: PageProps<"/confirm">) {
  return (
    <main className="narrow" id="view">
      <h1>Confirm your email</h1>
      <p>Press the button to start receiving Postpaper.</p>
      <Suspense fallback={null}>
        <Body searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
