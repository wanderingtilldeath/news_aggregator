// The placeholder a page shows while its data loads: a heading bar and three text bars with the site's
// shimmer (.sk in globals.css, still when the reader asks for less motion). It is what every
// <Suspense> around fetched content falls back to.
export default function Loading() {
  return (
    <div className="loading" role="status" aria-live="polite">
      <span className="sr-only">Loading…</span>
      <div className="sk sk-title" />
      <div className="sk sk-line" />
      <div className="sk sk-line" />
      <div className="sk sk-line" />
    </div>
  );
}
