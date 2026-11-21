"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";

// next/link leaves the scroll alone when the new page's first element is already in view, which on a short page
// (Notepad) can leave the reader below the top bars. Go to the top on every page change, unless the URL has a hash.
export default function ScrollTop() {
  const path = usePathname();
  useEffect(() => {
    if (!window.location.hash) window.scrollTo(0, 0);
  }, [path]);
  return null;
}
