import Link from "next/link";
import { Suspense } from "react";
import Logo from "./logo";
import { FOOTER } from "@/lib/menu";
import { ActiveNav, PlainNav } from "./primary-nav";
import MoreMenu from "./more-menu";
import ThemeToggle from "./theme-toggle";
import Today from "./today";

// The nameplate: today's date, the paper's name between rules, and a line about it. It scrolls
// away; the sections bar below it stays.
export function Masthead() {
  return (
    <header className="masthead">
      <div className="wrap">
        <div className="mast-row">
          <Today />
          <Logo size="lg" />
          <p className="tagline">
            The finished daily newspaper.
            <br />
            <Link href="/#subscribe">Get it free by email →</Link>
          </p>
        </div>
      </div>
    </header>
  );
}

// The sections, plus search and Subscribe. This bar is sticky, so they are always in reach.
export function SectionBar() {
  return (
    <div className="sectionbar">
      <div className="wrap bar">
        <nav className="primary-nav" aria-label="Sections">
          <Suspense fallback={<PlainNav />}>
            <ActiveNav />
          </Suspense>
        </nav>
        <MoreMenu />
        <form action="/" role="search" className="search">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
            <circle cx="7" cy="7" r="4.8" fill="none" stroke="currentColor" strokeWidth="1.6" />
            <path d="m10.6 10.6 3 3" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          </svg>
          <input type="search" name="q" placeholder="Search stories" aria-label="Search stories" />
        </form>
        <ThemeToggle />
        <Link href="/#subscribe" className="btn primary small">
          Subscribe
        </Link>
      </div>
    </div>
  );
}

export function SiteFooter() {
  return (
    <footer className="site-footer">
      <div className="wrap row-links">
        <Logo size="sm" />
        <nav aria-label="Footer" className="footer-cols">
          {FOOTER.map((g) => (
            <div key={g.title}>
              <h2>{g.title}</h2>
              <ul>
                {g.links.map((l) => (
                  <li key={l.href}>
                    <Link href={l.href}>{l.label}</Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </nav>
      </div>
      <p className="wrap fine">Every story page links to the original publisher. Summaries and the &ldquo;why it matters&rdquo; lines are generated from the article text.</p>
    </footer>
  );
}
