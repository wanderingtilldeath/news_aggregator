"use client";

import { useSyncExternalStore } from "react";

// Today's date for the dateline above the nameplate, in India time ("Sunday, 15 November 2026"). It
// is read in the browser only (the server renders an empty placeholder), so the static header never
// bakes in a date and the layout needs no request at build time.
const fmt = new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Kolkata", weekday: "long", day: "numeric", month: "long", year: "numeric" });

const subscribe = () => () => {};
const snapshot = () => fmt.format(new Date()).replace(/^(\w+)\s/, "$1, ");

export default function Today() {
  const value = useSyncExternalStore(subscribe, snapshot, () => "");

  return (
    <time className="today" suppressHydrationWarning>
      {value || "\u00a0"}
    </time>
  );
}
