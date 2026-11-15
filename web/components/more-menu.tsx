"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { MORE } from "@/lib/menu";

// The "More" menu in the sections bar: a button that opens a panel of the site's tools. It sits
// outside the scrolling list of sections so the panel is not clipped, closes on a click outside, on
// Escape and when a link is followed, and works with the keyboard like any disclosure button.
export default function MoreMenu() {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const outside = (e: MouseEvent) => {
      if (!box.current?.contains(e.target as Node)) setOpen(false);
    };
    const escape = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("mousedown", outside);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);

  return (
    <div className="more-menu" ref={box}>
      <button type="button" className="more-btn" aria-expanded={open} aria-controls="more-panel" onClick={() => setOpen(!open)}>
        More
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d="m1.5 3.5 3.5 3.5 3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open && (
        <div id="more-panel" className="more-panel">
          {MORE.map((g) => (
            <div key={g.title}>
              <h2>{g.title}</h2>
              <ul>
                {g.links.map((l) => (
                  <li key={l.href}>
                    <Link href={l.href} onClick={() => setOpen(false)}>
                      {l.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
