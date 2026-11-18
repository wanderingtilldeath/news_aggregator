import type { Metadata } from "next";
import Link from "next/link";
import Notebook from "@/components/notebook";

export const metadata: Metadata = { title: "Your notebook · Postpaper", description: "Sentences you saved from stories, kept in your browser, ready to print as a PDF.", robots: { index: false } };

export default function Page() {
  return (
    <main className="page" id="view">
      <h1>Your notebook</h1>
      <p className="lead-in">The sentences you saved from stories. They stay in this browser: nothing is sent to us, and clearing your browser data clears them.</p>
      <Notebook />
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}
