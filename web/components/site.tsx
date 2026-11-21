import Link from "next/link";
import { Suspense } from "react";
import Logo from "./logo";
import { FOOTER } from "@/lib/menu";
import { ActiveNav, PlainNav } from "./primary-nav";
import MoreMenu, { ActiveMoreMenu } from "./more-menu";
import PlayMenu, { ActivePlayMenu } from "./play-menu";
import CityWeather from "./city-weather";
import NotepadLink, { ActiveNotepadLink } from "./notepad-link";
import ThemeToggle from "./theme-toggle";
import Today from "./today";

// The nameplate: today's date, the paper's name between rules, and the reader's city and weather. It scrolls
// away; the sections bar below it stays.
export function Masthead() {
  return (
    <header className="masthead">
      <div className="wrap">
        <div className="mast-row">
          <Today />
          <Logo size="lg" />
          <div className="mast-right">
            <CityWeather />
          </div>
        </div>
      </div>
    </header>
  );
}

// The sections, plus search and the theme switch. This bar is sticky, so they are always in reach.
export function SectionBar() {
  return (
    <div className="sectionbar">
      <div className="wrap bar">
        <nav className="primary-nav" aria-label="Sections">
          <Suspense fallback={<PlainNav />}>
            <ActiveNav />
          </Suspense>
        </nav>
        <Suspense fallback={<PlayMenu />}>
          <ActivePlayMenu />
        </Suspense>
        <Suspense fallback={<MoreMenu />}>
          <ActiveMoreMenu />
        </Suspense>
        <form action="/" role="search" className="search">
          <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
            <circle cx="7" cy="7" r="4.8" fill="none" stroke="currentColor" strokeWidth="1.6" />
            <path d="m10.6 10.6 3 3" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
          </svg>
          <input type="search" name="q" placeholder="Search stories" aria-label="Search stories" />
        </form>
        <Suspense fallback={<NotepadLink />}>
          <ActiveNotepadLink />
        </Suspense>
        <ThemeToggle />
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
