"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { TRACKS, TRACK_IDS } from "@/lib/view";

const ITEMS = [
  { href: "/", label: "Home", match: (p: string, t: string) => p === "/" && !t },
  ...TRACK_IDS.map((id) => ({ href: `/?track=${id}`, label: id === "upsc" ? "UPSC" : TRACKS[id].title, match: (p: string, t: string) => p === "/" && t === id })),
  { href: "/quiz", label: "Quiz", match: (p: string) => p.startsWith("/quiz") },
  { href: "/archive", label: "Archive", match: (p: string) => p.startsWith("/archive") },
  { href: "/week", label: "Weekly revision", match: (p: string) => p.startsWith("/week") },
];

// The main navigation, with the current page marked. It reads the URL, so it sits in a
// <Suspense> boundary in the header; the same links render without the mark meanwhile.
export function ActiveNav() {
  const path = usePathname();
  const track = useSearchParams().get("track") ?? "";
  return (
    <>
      {ITEMS.map((it) => (
        <Link key={it.href} href={it.href} aria-current={it.match(path, track) ? "page" : undefined}>
          {it.label}
        </Link>
      ))}
    </>
  );
}

export function PlainNav() {
  return (
    <>
      {ITEMS.map((it) => (
        <Link key={it.href} href={it.href}>
          {it.label}
        </Link>
      ))}
    </>
  );
}
