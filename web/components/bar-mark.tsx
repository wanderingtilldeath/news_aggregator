"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Mark } from "./logo";

// A small bird at the left of the sticky sections bar, shown once the big nameplate has scrolled out of
// view, so the paper's name is never lost on a long page. It is always in the layout (its width animates),
// so nothing else in the bar jumps.
export default function BarMark() {
  const [on, setOn] = useState(false);

  useEffect(() => {
    const mast = document.querySelector(".masthead");
    if (!mast) return;
    const io = new IntersectionObserver(([e]) => setOn(!e.isIntersecting));
    io.observe(mast);
    return () => io.disconnect();
  }, []);

  return (
    <Link href="/" className={`bar-mark${on ? " on" : ""}`} aria-label="PostPaper home" tabIndex={on ? 0 : -1} aria-hidden={on ? undefined : true}>
      <Mark className="bar-mark-svg" />
    </Link>
  );
}
