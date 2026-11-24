"use client";

import { useEffect, useRef } from "react";
import SuggestInput from "./suggest-input";

// The search field of the sections bar. It is an icon until it is focused. Pressing "/" anywhere on the page
// (outside a text field) focuses it, and a faint "/" key shows inside the opened field as a reminder.
export default function SearchBox() {
  const input = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "/" || e.metaKey || e.ctrlKey || e.altKey) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.isContentEditable || /^(input|textarea|select)$/i.test(t.tagName))) return;
      e.preventDefault();
      input.current?.focus();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  return (
    <form action="/" role="search" className="search">
      <svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
        <circle cx="7" cy="7" r="4.8" fill="none" stroke="currentColor" strokeWidth="1.6" />
        <path d="m10.6 10.6 3 3" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
      <SuggestInput ref={input} type="search" name="q" placeholder="Search stories" aria-label="Search stories" aria-keyshortcuts="/" />
      <kbd className="search-key" aria-hidden="true">
        /
      </kbd>
    </form>
  );
}
