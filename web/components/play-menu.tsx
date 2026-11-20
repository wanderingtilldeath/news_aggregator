"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useRef, useState } from "react";
import { GAMES } from "@/lib/game-list";
import { useDismiss } from "./more-menu";

// The "Play" menu in the sections bar: a button that lists the daily games, so a game is one click
// away without the hub page. It looks and behaves like the More menu and marks itself on a games page.
export default function PlayMenu({ path = "" }: { path?: string }) {
  const [open, setOpen] = useState(false);
  const box = useRef<HTMLDivElement>(null);
  useDismiss(open, setOpen, box);
  const inGames = path === "/games" || path.startsWith("/games/");

  return (
    <div className="more-menu play-menu" ref={box}>
      <button type="button" className="more-btn" data-here={inGames ? "true" : undefined} aria-expanded={open} aria-controls="play-panel" onClick={() => setOpen(!open)}>
        Play
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d="m1.5 3.5 3.5 3.5 3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>
      {open && (
        <div id="play-panel" className="more-panel play-panel">
          <ul>
            {GAMES.map((g) => (
              <li key={g.href}>
                <Link href={g.href} aria-current={path === g.href ? "page" : undefined} onClick={() => setOpen(false)}>
                  {g.name}
                  <span>{g.text}</span>
                </Link>
              </li>
            ))}
          </ul>
          <Link href="/games" className="play-all" onClick={() => setOpen(false)}>
            All games
          </Link>
        </div>
      )}
    </div>
  );
}

// Told which page the reader is on; the path makes it dynamic, so it sits in a <Suspense> boundary.
export function ActivePlayMenu() {
  return <PlayMenu path={usePathname()} />;
}
