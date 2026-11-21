import Link from "next/link";
import Share from "./share";

// The close of the home page: the edition is read, so say when the next one comes and where to go from here.
export default function Pitch() {
  return (
    <section className="pitch" aria-labelledby="pitch-h">
      <div>
        <h2 id="pitch-h">See you tomorrow.</h2>
        <p>A new edition is ready every morning, ranked, with a plain line on why each story matters. Until then:</p>
        <nav className="pitch-links" aria-label="Explore more">
          <Link href="/quiz">Take the quiz</Link>
          <Link href="/flashcards">Revise flashcards</Link>
          <Link href="/notebook">Open your notepad</Link>
          <Link href="/concepts">Learn the concepts</Link>
        </nav>
      </div>
      <div className="pitch-actions">
        {/* A plain anchor: next/link skips the scroll when the hash is already #subscribe. */}
        <a className="btn primary" href="#subscribe">
          Get it by email
        </a>
        <Share compact path="/" title="Postpaper: the day's news in one minute, ranked, with why it matters" label="Share" />
      </div>
    </section>
  );
}
