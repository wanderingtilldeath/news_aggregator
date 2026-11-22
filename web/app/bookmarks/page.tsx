import type { Metadata } from "next";
import Link from "next/link";
import Bookmarks from "@/components/bookmarks";

export const metadata: Metadata = { title: "Bookmarks · Postpaper", description: "Stories you saved to read later, kept in your browser.", robots: { index: false } };

export default function Page() {
  return (
    <main className="page wide-list" id="view">
      <h1>Bookmarks</h1>
      <p className="lead-in">Stories you saved to read later. They stay in this browser: nothing is sent to us, and clearing your browser data clears them.</p>
      <Bookmarks />
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
