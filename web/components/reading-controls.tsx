"use client";

import { useSyncExternalStore } from "react";

const KEY = "pp-read";
const MIN = -1;
const MAX = 3;
const listeners = new Set<() => void>();

const subscribe = (fn: () => void) => {
  listeners.add(fn);
  return () => void listeners.delete(fn);
};

// The reading settings in force: data-rsize and data-rwidth on <html>, which the inline script in the layout
// sets from the saved choice before the page paints, and this control changes. The stylesheet reads them.
const snapshot = () => `${document.documentElement.dataset.rsize ?? "0"}|${document.documentElement.dataset.rwidth ?? ""}`;

// A−, A+ and a narrow or wide column for the story text, remembered in this browser.
export default function ReadingControls() {
  const [sizeText, width] = useSyncExternalStore(subscribe, snapshot, () => "0|").split("|");
  const size = Number(sizeText);

  function apply(nextSize: number, nextWidth: string) {
    const root = document.documentElement;
    if (nextSize) root.dataset.rsize = String(nextSize);
    else delete root.dataset.rsize;
    if (nextWidth) root.dataset.rwidth = nextWidth;
    else delete root.dataset.rwidth;
    try {
      localStorage.setItem(KEY, JSON.stringify({ s: nextSize, w: nextWidth }));
    } catch {
      // the choice just is not remembered
    }
    listeners.forEach((fn) => fn());
  }

  return (
    <div className="read-controls" role="group" aria-label="Reading settings">
      <button type="button" onClick={() => apply(size - 1, width)} disabled={size <= MIN} aria-label="Smaller text" title="Smaller text">
        A<span aria-hidden="true">−</span>
      </button>
      <button type="button" onClick={() => apply(size + 1, width)} disabled={size >= MAX} aria-label="Larger text" title="Larger text">
        A<span aria-hidden="true">+</span>
      </button>
      <span className="rc-sep" aria-hidden="true" />
      <button type="button" aria-pressed={width === "n"} onClick={() => apply(size, width === "n" ? "" : "n")} title="A narrow column">
        Narrow
      </button>
      <button type="button" aria-pressed={width === "w"} onClick={() => apply(size, width === "w" ? "" : "w")} title="A wide column">
        Wide
      </button>
    </div>
  );
}
