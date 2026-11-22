"use client";

import { useEffect, useState } from "react";

// Two buttons fixed to the corner on every page: "Get it by email" the whole way down (hidden while the signup band
// itself is on screen) and "Back to top" once the reader has scrolled a screen or so.
export default function FloatingActions() {
  const [scrolled, setScrolled] = useState(false);
  const [bandInView, setBandInView] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 600);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  // The band is not on every page, and arrives with client navigation, so look for it again on each scroll-free tick.
  useEffect(() => {
    let io: IntersectionObserver | undefined;
    let target: Element | null = null;
    const watch = () => {
      const el = document.getElementById("subscribe");
      if (el === target) return;
      io?.disconnect();
      target = el;
      setBandInView(false);
      if (!el) return;
      io = new IntersectionObserver(([e]) => setBandInView(e.isIntersecting));
      io.observe(el);
    };
    watch();
    const mo = new MutationObserver(watch);
    mo.observe(document.body, { childList: true, subtree: true });
    return () => {
      mo.disconnect();
      io?.disconnect();
    };
  }, []);

  const top = () => window.scrollTo({ top: 0, behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });

  return (
    <div className="float-actions">
      {scrolled && (
        <button type="button" className="btn small float-top" onClick={top}>
          <span aria-hidden="true">↑</span> Back to top
        </button>
      )}
      {!bandInView && (
        // A plain link on purpose: next/link does not scroll again when the hash is already in the address.
        // eslint-disable-next-line @next/next/no-html-link-for-pages
        <a className="btn primary small" href="/#subscribe">
          Get it by email
        </a>
      )}
    </div>
  );
}
