// The header's weather is looked up from the reader's connection by /api/weather, a function call each time. Keeping the
// answer in the browser for half an hour makes it one call per reader per half hour, not one per page.

export type CachedWeather = { city?: string; temp?: number; code?: number };

export const WEATHER_KEY = "pp-weather";
export const WEATHER_FRESH_MS = 30 * 60 * 1000;

// freshWeather returns the stored answer when it is under half an hour old and has a city and a temperature, else null
// (nothing stored, malformed, stale, or from a clock that has moved backwards).
export function freshWeather(raw: string, now: number): CachedWeather | null {
  try {
    const { at, body } = JSON.parse(raw || "null") ?? {};
    if (typeof at !== "number" || now - at < 0 || now - at >= WEATHER_FRESH_MS) return null;
    if (!body || typeof body.city !== "string" || !body.city || typeof body.temp !== "number") return null;

    return { city: body.city, temp: body.temp, code: typeof body.code === "number" ? body.code : 0 };
  } catch {
    return null;
  }
}

export const packWeather = (body: CachedWeather, now: number) => JSON.stringify({ at: now, body });
