import type { NewsEvent } from "./api";

// An iCalendar file (RFC 5545) of the upcoming events, so a reader can subscribe to them in any
// calendar app. Text is escaped, lines are folded at 75 bytes, and each event is an all-day entry
// with a stable UID, so a calendar updates an event instead of duplicating it.

const esc = (s: string) => s.replace(/\\/g, "\\\\").replace(/;/g, "\;").replace(/,/g, "\\,").replace(/\r?\n/g, "\\n");

// Folds a line at 75 bytes without splitting a character: the rest goes on a new line that starts with a space.
function fold(line: string) {
  const enc = new TextEncoder();
  const out: string[] = [];
  let cur = "";
  let bytes = 0;
  for (const ch of line) {
    const n = enc.encode(ch).length;
    if (bytes + n > (out.length ? 74 : 75)) {
      out.push(cur);
      cur = "";
      bytes = 0;
    }
    cur += ch;
    bytes += n;
  }
  out.push(cur);

  return out.join("\r\n ");
}

const compact = (day: string) => day.replace(/-/g, "");

function nextDay(day: string) {
  const d = new Date(`${day}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + 1);

  return d.toISOString().slice(0, 10);
}

function uid(e: NewsEvent) {
  let h = 5381;
  for (const ch of e.text) h = ((h * 33) ^ ch.charCodeAt(0)) >>> 0;

  return `${e.story}-${e.day}-${h.toString(36)}@postpaper.in`;
}

export function buildICS(events: NewsEvent[], siteURL: string, now: Date) {
  const stamp = now.toISOString().replace(/[-:]/g, "").replace(/\.\d+/, "");
  const lines = ["BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Postpaper//Upcoming//EN", "CALSCALE:GREGORIAN", "X-WR-CALNAME:Postpaper: upcoming"];
  for (const e of events) {
    const summary = e.text.length > 80 ? e.text.slice(0, e.text.lastIndexOf(" ", 80)).replace(/[,;:\s]+$/, "") + "…" : e.text;
    lines.push(
      "BEGIN:VEVENT",
      `UID:${uid(e)}`,
      `DTSTAMP:${stamp}`,
      `DTSTART;VALUE=DATE:${compact(e.day)}`,
      `DTEND;VALUE=DATE:${compact(nextDay(e.day))}`,
      `SUMMARY:${esc(summary)}`,
      `DESCRIPTION:${esc(`${e.text}\n\nFrom: ${e.title} (${e.source}). Check the story before you rely on the date.`)}`,
      `URL:${siteURL}/story/${encodeURIComponent(e.story)}`,
      "END:VEVENT",
    );
  }
  lines.push("END:VCALENDAR");

  return lines.map(fold).join("\r\n") + "\r\n";
}
