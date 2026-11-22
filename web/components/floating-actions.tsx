"use client";

import { useEffect, useState } from "react";

// One button fixed to the corner of every page: "Back to top", once the reader has scrolled a screen or so. It means
// the same on a story, a table of figures or the admin dashboard. (The "Get it by email" button used to float here
// too, and made no sense on the admin, account and log-in pages; that call to action now lives where it belongs: the
// masthead, the signup band and the footer.)
export default function FloatingActions() {
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 600);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const top = () => window.scrollTo({ top: 0, behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });

  if (!scrolled) return null;

  return (
    <div className="float-actions">
      <button type="button" className="btn small float-top" onClick={top}>
        <span aria-hidden="true">↑</span> Back to top
      </button>
    </div>
  );
}
