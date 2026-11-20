"use client";

import { useEffect, useRef } from "react";

// A thin line at the top of a story page that fills as the reader scrolls down it. It sets the width
// directly (no state), once per animation frame, so scrolling stays smooth.
export default function ReadProgress() {
  const bar = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let frame = 0;
    const update = () => {
      frame = 0;
      const page = document.documentElement;
      const room = page.scrollHeight - page.clientHeight;
      if (bar.current) bar.current.style.width = room > 0 ? `${Math.min(100, (window.scrollY / room) * 100)}%` : "0";
    };
    const onScroll = () => {
      if (!frame) frame = requestAnimationFrame(update);
    };
    update();
    window.addEventListener("scroll", onScroll, { passive: true });
    window.addEventListener("resize", onScroll);
    return () => {
      window.removeEventListener("scroll", onScroll);
      window.removeEventListener("resize", onScroll);
      if (frame) cancelAnimationFrame(frame);
    };
  }, []);

  return (
    <div className="read-progress" aria-hidden="true">
      <div ref={bar} />
    </div>
  );
}
