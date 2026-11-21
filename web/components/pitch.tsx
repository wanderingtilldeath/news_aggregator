import Link from "next/link";
import Share from "./share";

// The one-line case for Postpaper, with the way to get it and a way to pass it on, at the top of the front page.
// The signup band further down is where #subscribe lands.
export default function Pitch() {
  return (
    <section className="pitch" aria-label="About Postpaper">
      <p>
        <b>The day&rsquo;s news in one minute.</b> A free daily edition: the top stories ranked, with a plain line on why each one matters. Made for busy readers, UPSC aspirants and engineers.
      </p>
      <div className="pitch-actions">
        <Link className="btn primary" href="#subscribe">
          Get it by email
        </Link>
        <Share path="/" title="Postpaper: the day's news in one minute, ranked, with why it matters" label="Share today's edition" />
      </div>
    </section>
  );
}
