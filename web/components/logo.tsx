import Link from "next/link";

// The Postpaper origami bird, redrawn as SVG from the brand logo (six facets), so it stays sharp
// at any size and can follow the theme: the darkest facet lightens on a dark page.
export function Mark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="90 190 265 265" aria-hidden="true" focusable="false">
      <polygon points="97,196 210,232 188,332" fill="#049fe3" />
      <polygon points="210,232 288,333 188,332" fill="#3a2d9b" />
      <polygon points="303,232 246,279 289,332" fill="#1e88d8" />
      <polygon points="303,232 348,262 299,268" fill="#3e2c9e" />
      <polygon points="188,332 288,333 205,393" fill="var(--logo-navy)" />
      <polygon points="188,332 205,393 134,448" fill="#3b2e9c" />
    </svg>
  );
}

// The full logo: the bird and the name. It links home.
export default function Logo({ size = "md" }: { size?: "sm" | "md" | "lg" }) {
  return (
    <Link href="/" className={`logo logo-${size}`} aria-label="PostPaper home">
      <Mark className="logo-mark" />
      <span className="logo-text">PostPaper</span>
    </Link>
  );
}
