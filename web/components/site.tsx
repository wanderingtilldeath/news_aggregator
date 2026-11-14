import Link from "next/link";
import { Suspense } from "react";
import { ActiveNav, PlainNav } from "./primary-nav";

// The header and footer shared by every page. The header itself is static; only the
// navigation's current-page mark reads the URL.
export function SiteHeader() {
  return (
    <header className="site">
      <div className="wrap bar">
        <Link href="/" className="logo" aria-label="Postpaper home">
          <span className="mark" aria-hidden="true">
            P
          </span>
          Postpaper
        </Link>
        <nav className="primary-nav" aria-label="Main">
          <Suspense fallback={<PlainNav />}>
            <ActiveNav />
          </Suspense>
        </nav>
        <form action="/" role="search" className="search">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
            <circle cx="7" cy="7" r="4.8" fill="none" stroke="currentColor" strokeWidth="1.6" />
            <path d="m10.6 10.6 3 3" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          </svg>
          <input type="search" name="q" placeholder="Search stories" aria-label="Search stories" />
        </form>
        <Link href="/#subscribe" className="btn primary">
          Subscribe
        </Link>
      </div>
    </header>
  );
}

export function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="wrap row-links">
        <Link href="/" className="logo">
          <span className="mark" aria-hidden="true">
            P
          </span>
          Postpaper
        </Link>
        <nav aria-label="Footer">
          <Link href="/archive">Archive</Link>
          <Link href="/week">Weekly revision</Link>
          <Link href="/quiz">Quiz</Link>
          <Link href="/#subscribe">Get it by email</Link>
        </nav>
      </div>
      <p className="wrap fine">Every headline links to the original publisher. Summaries and the &ldquo;why it matters&rdquo; lines are generated from the article text.</p>
    </footer>
  );
}
