"use client";

import { useSyncExternalStore } from "react";

const KEY = "pp-theme";
const listeners = new Set<() => void>();

// The theme in force: data-theme on <html>, which the layout sets to dark and its inline script changes to
// light for a reader who chose light, and which this switch changes.
function effective(): "light" | "dark" {
  return document.documentElement.dataset.theme === "light" ? "light" : "dark";
}

function subscribe(fn: () => void) {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

// Switches between light and dark and remembers the choice in this browser. The page opens in dark.
export default function ThemeToggle() {
  const theme = useSyncExternalStore(subscribe, effective, () => "dark");
  const next = theme === "dark" ? "light" : "dark";

  function flip() {
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem(KEY, next);
    } catch {
      // the choice just is not remembered
    }
    listeners.forEach((fn) => fn());
  }

  return (
    <button type="button" className="theme-toggle" onClick={flip} aria-label={`Switch to ${next} mode`} title={`Switch to ${next} mode`}>
      {theme === "dark" ? (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" aria-hidden="true">
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2.5v2.2M12 19.3v2.2M2.5 12h2.2M19.3 12h2.2M5.3 5.3l1.6 1.6M17.1 17.1l1.6 1.6M5.3 18.7l1.6-1.6M17.1 6.9l1.6-1.6" />
        </svg>
      ) : (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="M20.5 14.2A8.5 8.5 0 0 1 9.8 3.5a8.5 8.5 0 1 0 10.7 10.7Z" />
        </svg>
      )}
    </button>
  );
}
