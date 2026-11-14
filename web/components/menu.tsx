"use client";

import { useEffect, useRef } from "react";

// A dropdown built on <details>, so it works without script and is keyboard accessible.
// The script only adds what <details> lacks: it closes on an outside click, on Escape, and
// when a link inside is chosen.
export default function Menu({ label, value, children }: { label: string; value: string; children: React.ReactNode }) {
  const ref = useRef<HTMLDetailsElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const close = () => el.removeAttribute("open");
    const onDown = (e: MouseEvent) => {
      if (el.open && !el.contains(e.target as Node)) close();
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && el.open) {
        close();
        el.querySelector("summary")?.focus();
      }
    };
    const onClick = (e: MouseEvent) => {
      if ((e.target as HTMLElement).closest("a")) close();
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    el.addEventListener("click", onClick);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
      el.removeEventListener("click", onClick);
    };
  }, []);

  return (
    <details className="menu" ref={ref}>
      <summary aria-label={`${label}: ${value}`}>
        <span className="k">{label}</span> {value}
        <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
          <path d="M2.5 4.5 6 8l3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </summary>
      <div className="menu-pop">{children}</div>
    </details>
  );
}
