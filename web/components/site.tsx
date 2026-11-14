import Link from "next/link";
import { TRACKS, TRACK_IDS } from "@/lib/view";

// The header and footer shared by every page. Static: nothing here reads the request.
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
          <Link href="/">Home</Link>
          {TRACK_IDS.map((t) => (
            <Link key={t} href={`/?track=${t}`}>
              {TRACKS[t].title}
            </Link>
          ))}
          <Link href="/quiz">Quiz</Link>
          <Link href="/archive">Archive</Link>
          <Link href="/week">Weekly revision</Link>
        </nav>
        <form action="/" role="search" className="search">
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
      <div className="wrap cols">
        <div>
          <Link href="/" className="logo">
            <span className="mark" aria-hidden="true">
              P
            </span>
            Postpaper
          </Link>
          <p>The finished daily newspaper for UPSC aspirants and engineers. One edition a day, then you are done.</p>
        </div>
        <div>
          <h3>Editions</h3>
          <ul>
            {TRACK_IDS.map((t) => (
              <li key={t}>
                <Link href={`/?track=${t}`}>{TRACKS[t].title}</Link>
              </li>
            ))}
          </ul>
        </div>
        <div>
          <h3>Explore</h3>
          <ul>
            <li>
              <Link href="/quiz">Daily quiz</Link>
            </li>
            <li>
              <Link href="/archive">Archive</Link>
            </li>
            <li>
              <Link href="/week">Weekly revision</Link>
            </li>
            <li>
              <Link href="/#subscribe">Get it by email</Link>
            </li>
          </ul>
        </div>
        <div>
          <h3>Reading</h3>
          <ul>
            <li>
              <Link href="/?mode=1">1-minute read</Link>
            </li>
            <li>
              <Link href="/?mode=7">7-minute read</Link>
            </li>
          </ul>
        </div>
      </div>
      <div className="wrap fine">Every headline links to the original publisher. Summaries and the &ldquo;why it matters&rdquo; lines are generated from the article text.</div>
    </footer>
  );
}
