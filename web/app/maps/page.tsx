import type { Metadata } from "next";
import MapPractice from "@/components/map-practice";

export const metadata: Metadata = {
  title: "Map practice · Postpaper",
  description: "Mark places on a map of India and write notes on them. Ready-made layers for soils, climate, tiger reserves, biosphere reserves, rivers, passes and ports.",
};

export default function Page() {
  return (
    <main className="content np-page" id="view">
      <MapPractice />
    </main>
  );
}
