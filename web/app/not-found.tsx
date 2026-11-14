import Link from "next/link";

// Shown for a story that is gone (pages are kept about 60 days) or any address that does not exist.
export default function NotFound() {
  return (
    <main className="content" id="view">
      <div className="empty-card" style={{ maxWidth: 560, margin: "var(--s8) auto" }}>
        <h2>We could not find that page</h2>
        <p>The story may be older than 60 days, or the address is wrong.</p>
        <Link className="btn primary" href="/">
          Back to today&rsquo;s edition
        </Link>
      </div>
    </main>
  );
}
