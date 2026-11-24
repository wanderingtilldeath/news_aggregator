"use client";

import { useEffect, useState } from "react";

// Printing a sheet that is not on screen: `print` shows it (the caller puts <MapSheet> in a portal while `printing`), sets
// data-print on <html> so the stylesheet hides everything else (.mp-sheet in globals.css), opens the print dialog and puts
// everything back when it closes.
export function usePrintSheet(): [boolean, () => void] {
  const [printing, setPrinting] = useState(false);

  useEffect(() => {
    if (!printing) return;
    const root = document.documentElement;
    const done = () => {
      delete root.dataset.print;
      setPrinting(false);
    };
    root.dataset.print = "blank";
    window.addEventListener("afterprint", done, { once: true });
    window.print();
    return () => {
      window.removeEventListener("afterprint", done);
      delete root.dataset.print;
    };
  }, [printing]);

  return [printing, () => setPrinting(true)];
}
