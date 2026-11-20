"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";

// The sections bar. The sections are the story categories, the way a newspaper has World,
// Business and Sport; the page you are on is marked. The tools (quiz, games, archive and the
// rest) live in the More menu, so the bar stays short.
const SECTIONS: { href: string; label: string; category: string; prefix?: string }[] = [
  { href: "/", label: "Top stories", category: "" },
  { href: "/?category=india", label: "India", category: "india" },
  { href: "/?category=world", label: "World", category: "world" },
  { href: "/?category=upsc", label: "UPSC", category: "upsc" },
  { href: "/?category=tech", label: "Tech", category: "tech" },
  { href: "/?category=ai", label: "AI", category: "ai" },
  // A page of its own (the funding table), not a filter of the front page, so it is matched by path.
  { href: "/funding", label: "Startups", category: "", prefix: "/funding" },
];

// It reads the URL, so it sits in a <Suspense> boundary in the header; the same links render
// without the mark meanwhile.
export function ActiveNav() {
  const path = usePathname();
  const params = useSearchParams();
  const category = params.get("category") ?? "";
  const onHome = path === "/" && !params.get("q");
  return (
    <>
      {SECTIONS.map((s) => (
        <Link key={s.href} href={s.href} aria-current={(s.prefix ? path.startsWith(s.prefix) : onHome && category === s.category) ? "page" : undefined}>
          {s.label}
        </Link>
      ))}
    </>
  );
}

export function PlainNav() {
  return (
    <>
      {SECTIONS.map((s) => (
        <Link key={s.href} href={s.href}>
          {s.label}
        </Link>
      ))}
    </>
  );
}
