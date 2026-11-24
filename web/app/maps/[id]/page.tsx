import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import MapViewer from "@/components/map-viewer";
import { kindLabel } from "@/lib/map-catalog";
import world from "@/lib/data/world.json";
import { MAPS } from "@/lib/maps-registry";

export function generateStaticParams() {
  return Object.keys(MAPS).map((id) => ({ id }));
}

export async function generateMetadata({ params }: PageProps<"/maps/[id]">): Promise<Metadata> {
  const { id } = await params;
  const map = MAPS[id];
  if (!map) return { title: "Map not found · Postpaper", robots: { index: false } };
  return { title: `${map.title}: map for revision · Postpaper`, description: map.blurb };
}

export default async function Page({ params }: PageProps<"/maps/[id]">) {
  const { id } = await params;
  const map = MAPS[id];
  if (!map) notFound();

  return (
    <main className="content np-page" id="view">
      <nav className="crumbs" aria-label="Breadcrumb">
        <Link href="/maps">Maps</Link>
        <span aria-hidden="true">/</span>
        <span>{kindLabel(map.category)}</span>
      </nav>
      <MapViewer map={map} world={map.base === "world" ? world : undefined} />
    </main>
  );
}
