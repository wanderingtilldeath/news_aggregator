import type { Metadata } from "next";
import Notebook from "@/components/notebook";

export const metadata: Metadata = { title: "Notepad · Postpaper", description: "The sentences you saved from stories, kept in your browser (or synced with Pro), ready to print as a PDF.", robots: { index: false } };

export default function Page() {
  return (
    <main className="content np-page" id="view">
      <Notebook />
    </main>
  );
}
