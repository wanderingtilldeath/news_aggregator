"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { MORE } from "@/lib/menu";
import SuggestInput from "./suggest-input";

// Closes an open panel on a click outside its box and on Escape.
export function useDismiss(open: boolean, setOpen: (v: boolean) => void, box: React.RefObject<HTMLElement | null>) {
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
  }, [open, setOpen, box]);
}

// The "More" menu in the sections bar: a button that opens a panel of the site's tools. It sits
// outside the scrolling list of sections so the panel is not clipped, closes on a click outside, on
// Escape and when a link is followed, and works with the keyboard like any disclosure button.
export default function MoreMenu({ path = "" }: { path?: string }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  // The bar marks the section you are in; the tools live in this menu, so it marks itself when you are on one.
  const here = (href: string) => path === href || path.startsWith(href + "/");
  const inMenu = MORE.some((g) => g.links.some((l) => here(l.href)));

  useDismiss(open, setOpen, box);

  return (
    <div className="more-menu" ref={box}>
      <button type="button" className="more-btn" data-here={inMenu ? "true" : undefined} aria-expanded={open} aria-controls="more-panel" onClick={() => setOpen(!open)}>
        More
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d="m1.5 3.5 3.5 3.5 3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open && (
        <div id="more-panel" className="more-panel">
          {/* The search box in the bar is hidden on a phone; this one takes its place there. */}
          <form action="/" role="search" className="more-search">
            <label className="sr-only" htmlFor="more-q">
              Search the news
            </label>
            <SuggestInput id="more-q" type="search" name="q" placeholder="Search the news" />
          </form>
          {MORE.map((g) => (
            <div key={g.title}>
              <h2>{g.title}</h2>
              <ul>
                {g.links.map((l) => (
                  <li key={l.href}>
                    <Link href={l.href} aria-current={here(l.href) ? "page" : undefined} onClick={() => {
                      setOpen(false);
                      // next/link does not re-scroll when the hash is unchanged.
                      if (path === "/" && l.href.startsWith("/#")) document.getElementById(l.href.slice(2))?.scrollIntoView();
                    }}>
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

// The same menu, told which page the reader is on. Reading the path makes it dynamic, so it sits in a
// <Suspense> boundary in the header with the unmarked menu as its fallback.
export function ActiveMoreMenu() {
  return <MoreMenu path={usePathname()} />;
}
