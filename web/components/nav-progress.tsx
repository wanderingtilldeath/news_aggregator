"use client";

import { usePathname, useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";

// A thin bar across the top of the page from the moment an internal link is pressed until the new page is there.
// It says "yes, that worked" on pages that take a moment to fetch, wherever a link is, without each link knowing.
// It starts on a click and ends when the address (path and query) changes to the one clicked; a timer ends it anyway
// if the navigation fails or is cancelled.
const GIVE_UP_MS = 12_000;

export default function NavProgress() {
  const here = usePathname() + "?" + useSearchParams().toString();
  const [going, setGoing] = useState("");

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const onClick = (e: MouseEvent) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = (e.target as Element | null)?.closest?.("a");
      if (!a || (a.target && a.target !== "_self") || a.hasAttribute("download")) return;
      const url = new URL(a.href, window.location.href);
      if (url.origin !== window.location.origin || (url.pathname === window.location.pathname && url.search === window.location.search)) return;
      setGoing(url.pathname + "?" + url.search.slice(1));
      clearTimeout(timer);
      timer = setTimeout(() => setGoing(""), GIVE_UP_MS);
    };
    document.addEventListener("click", onClick, true);
    return () => {
      document.removeEventListener("click", onClick, true);
      clearTimeout(timer);
    };
  }, []);

  // Active while the address is not yet the one that was clicked.
  if (!going || going === here) return null;

  return <div className="nav-progress" role="progressbar" aria-label="Loading the page" />;
}
