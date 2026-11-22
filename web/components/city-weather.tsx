"use client";

import { useEffect, useState } from "react";
import { readLocal, writeLocal } from "@/lib/local";
import { WEATHER_KEY, freshWeather, packWeather } from "@/lib/weather-cache";

type Weather = { city?: string; temp?: number; code?: number };

// The WMO weather codes Open-Meteo returns, grouped into the seven pictures below.
function kind(code: number): "sun" | "partly" | "cloud" | "fog" | "rain" | "snow" | "storm" {
  if (code <= 1) return "sun";
  if (code === 2) return "partly";
  if (code === 3) return "cloud";
  if (code === 45 || code === 48) return "fog";
  if ((code >= 71 && code <= 77) || code === 85 || code === 86) return "snow";
  if (code >= 95) return "storm";
  return "rain";
}

const CLOUD = "M7 18h10a4 4 0 0 0 .6-7.95A5.5 5.5 0 0 0 7 9.5 4.25 4.25 0 0 0 7 18Z";

function Icon({ code }: { code: number }) {
  const k = kind(code);
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      {k === "sun" && (
        <>
          <circle cx="12" cy="12" r="4" />
          <path d="M12 2.5v2M12 19.5v2M2.5 12h2M19.5 12h2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M5.3 18.7l1.4-1.4M17.3 6.7l1.4-1.4" />
        </>
      )}
      {k === "partly" && (
        <>
          <path d="M8.5 4.5v1.2M3.9 8.5H2.7M5.2 5.2l.9.9M12.2 5.2l-.9.9" />
          <path d="M7 12.2a3.2 3.2 0 1 1 5.9-1.4" />
          <path d="M9 19h8.2a3.3 3.3 0 0 0 .5-6.55A4.6 4.6 0 0 0 9 12.3 3.35 3.35 0 0 0 9 19Z" />
        </>
      )}
      {(k === "cloud" || k === "fog" || k === "rain" || k === "snow" || k === "storm") && <path d={CLOUD} />}
      {k === "fog" && <path d="M6 21h12" />}
      {k === "rain" && <path d="M9 20.5l.8-2M13 20.5l.8-2M17 20.5l.8-2" />}
      {k === "snow" && <path d="M9 20h.01M13 21h.01M17 20h.01" strokeWidth="2.4" />}
      {k === "storm" && <path d="m12 14-2 3.5h3L11.5 21" />}
    </svg>
  );
}

// The reader's city and its temperature in the masthead, found from their connection by /api/weather. It shows
// nothing until the answer is in, and nothing at all when the city is unknown.
export default function CityWeather() {
  const [w, setW] = useState<Weather>({});

  useEffect(() => {
    let live = true;
    // Half an hour of the last answer is kept in this browser, so a page view is not a function call.
    const kept = freshWeather(readLocal(WEATHER_KEY), Date.now());
    const answer: Promise<Weather> = kept
      ? Promise.resolve(kept)
      : fetch("/api/weather")
          .then((r) => (r.ok ? r.json() : {}))
          .then((body: Weather) => {
            if (live && body.city && typeof body.temp === "number") writeLocal(WEATHER_KEY, packWeather(body, Date.now()));
            return body;
          });
    answer.then((body) => live && setW(body)).catch(() => undefined);
    return () => {
      live = false;
    };
  }, []);

  if (!w.city || typeof w.temp !== "number") return null;
  return (
    <p className="cityw" title={`Weather in ${w.city}`}>
      <b>{w.city}</b>
      <Icon code={w.code ?? 0} />
      <span>{w.temp}°C</span>
    </p>
  );
}
