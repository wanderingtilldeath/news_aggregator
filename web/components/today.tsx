"use client";

import { useSyncExternalStore } from "react";

// Today's date for the masthead: a big day number beside the weekday, month and year, in India
// time. It is read in the browser only (the server renders an empty placeholder), so the static
// header never bakes in a date and the layout needs no request at build time.
const fmt = new Intl.DateTimeFormat("en-GB", { timeZone: "Asia/Kolkata", weekday: "long", day: "numeric", month: "long", year: "numeric" });

const subscribe = () => () => {};
const snapshot = () => {
  const parts = Object.fromEntries(fmt.formatToParts(new Date()).map((p) => [p.type, p.value]));
  return `${parts.day}|${parts.weekday}|${parts.month} ${parts.year}`;
};

export default function Today() {
  const value = useSyncExternalStore(subscribe, snapshot, () => "");
  const [n, weekday, monthYear] = value.split("|");

  return (
    <div className="today" aria-label={value ? `${weekday}, ${n} ${monthYear}` : undefined}>
      <span className="today-n" aria-hidden="true">
        {n || "\u00a0"}
      </span>
      <span className="today-t" aria-hidden="true">
        {weekday}
        <br />
        {monthYear}
      </span>
    </div>
  );
}
