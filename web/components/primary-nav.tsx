"use client";

import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";

// The sections bar. The sections are the story categories, the way a newspaper has World,
// Business and Sport; the page you are on is marked. The three tools sit after a divider.
const SECTIONS: { href: string; label: string; category: string }[] = [
  { href: "/", label: "Top stories", category: "" },
  { href: "/?category=india", label: "India", category: "india" },
  { href: "/?category=world", label: "World", category: "world" },
  { href: "/?category=upsc", label: "UPSC", category: "upsc" },
  { href: "/?category=tech", label: "Tech", category: "tech" },
  { href: "/?category=ai", label: "AI", category: "ai" },
];
const TOOLS = [
  { href: "/quiz", label: "Quiz", prefix: "/quiz" },
  { href: "/archive", label: "Archive", prefix: "/archive" },
  { href: "/week", label: "Weekly", prefix: "/week" },
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
        <Link key={s.href} href={s.href} aria-current={onHome && category === s.category ? "page" : undefined}>
          {s.label}
        </Link>
      ))}
      <span className="divider" aria-hidden="true" />
      {TOOLS.map((t) => (
        <Link key={t.href} href={t.href} aria-current={path.startsWith(t.prefix) ? "page" : undefined}>
          {t.label}
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
      <span className="divider" aria-hidden="true" />
      {TOOLS.map((t) => (
        <Link key={t.href} href={t.href}>
          {t.label}
        </Link>
      ))}
    </>
  );
}
