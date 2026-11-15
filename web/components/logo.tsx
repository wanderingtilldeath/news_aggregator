import Link from "next/link";

// The Postpaper origami bird, redrawn as SVG from the brand logo (twelve facets), so it stays sharp
// at any size and can follow the theme: the two darkest facets lighten on a dark page.
export function Mark({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="96 195 270 270" aria-hidden="true" focusable="false">
      <polygon points="101,241 146,246 205,337" fill="var(--logo-dark)" stroke="var(--logo-dark)" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="125,208 232,243 166,283" fill="#e42944" stroke="#e42944" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="232,243 166,283 207,340" fill="#ea6235" stroke="#ea6235" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="232,243 305,343 207,340" fill="#d32137" stroke="#d32137" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="207,339 229,403 160,452" fill="#d32137" stroke="#d32137" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="176,414 197,427 160,452" fill="#ea3345" stroke="#ea3345" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="260,281 318,279 305,343" fill="#d08821" stroke="#d08821" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="322,244 262,282 316,301 318,278" fill="#eeba49" stroke="#eeba49" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="324,246 362,268 318,280" fill="#ed3434" stroke="#ed3434" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="321,245 318,279 337,271" fill="var(--logo-dark)" stroke="var(--logo-dark)" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="208,340 305,343 258,381" fill="#d08821" stroke="#d08821" strokeWidth=".7" strokeLinejoin="round" />
      <polygon points="208,340 258,381 228,402" fill="#eeb848" stroke="#eeb848" strokeWidth=".7" strokeLinejoin="round" />
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
