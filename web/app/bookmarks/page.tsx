import type { Metadata } from "next";
import Bookmarks from "@/components/bookmarks";

export const metadata: Metadata = { title: "Bookmarks · Postpaper", description: "Stories you saved to read later, kept in your browser, or on your account when you are logged in.", robots: { index: false } };

export default function Page() {
  return (
    <main className="content np-page" id="view">
      <Bookmarks />
    </main>
  );
}
