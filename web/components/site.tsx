import Link from "next/link";
import { Suspense } from "react";
import Logo from "./logo";
import { ActiveNav, PlainNav } from "./primary-nav";
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
        <nav aria-label="Footer">
          <Link href="/?track=upsc">UPSC edition</Link>
          <Link href="/?track=engineering">Engineering edition</Link>
          <Link href="/quiz">Quiz</Link>
          <Link href="/flashcards">Flashcards</Link>
          <Link href="/notebook">Notebook</Link>
          <Link href="/epaper">E-paper</Link>
          <Link href="/upcoming">Upcoming</Link>
          <Link href="/concepts">Concepts</Link>
          <Link href="/graph">Who is in the news</Link>
          <Link href="/teams">Slack and Discord</Link>
          <Link href="/archive">Archive</Link>
          <Link href="/week">Weekly revision</Link>
          <Link href="/month">Monthly compilations</Link>
          <Link href="/underreported">Underreported</Link>
          <Link href="/pro">Pro</Link>
          <Link href="/sources">Sources and takedown</Link>
          <Link href="/#subscribe">Get it by email</Link>
        </nav>
      </div>
      <p className="wrap fine">Every story page links to the original publisher. Summaries and the &ldquo;why it matters&rdquo; lines are generated from the article text.</p>
    </footer>
  );
}
