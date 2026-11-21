// The reader's city and its weather for the masthead ("Bengaluru, 27°C"). The city comes from the request's IP
// address, which Vercel's edge resolves into headers; nothing is stored and the address is not passed on. The
// weather is Open-Meteo's, which needs no key, asked for a coordinate rounded to a city and kept for 30 minutes.
// Anywhere the headers are missing (a local run) the answer is empty and the masthead shows nothing.
const FORECAST = process.env.WEATHER_URL ?? "https://api.open-meteo.com/v1/forecast";

export async function GET(req: Request) {
  const none = () => Response.json({}, { headers: { "Cache-Control": "private, max-age=600" } });

  let city = "";
  try {
    city = decodeURIComponent(req.headers.get("x-vercel-ip-city") ?? "").slice(0, 60);
  } catch {
    return none();
  }
  const lat = Number(req.headers.get("x-vercel-ip-latitude"));
  const lon = Number(req.headers.get("x-vercel-ip-longitude"));
  if (!city || !Number.isFinite(lat) || !Number.isFinite(lon) || (lat === 0 && lon === 0) || Math.abs(lat) > 90 || Math.abs(lon) > 180) return none();

  try {
    const res = await fetch(`${FORECAST}?latitude=${lat.toFixed(1)}&longitude=${lon.toFixed(1)}&current=temperature_2m,weather_code`, {
      next: { revalidate: 1800 },
      signal: AbortSignal.timeout(5000),
    });
    if (!res.ok) return none();
    const body = (await res.json()) as { current?: { temperature_2m?: number; weather_code?: number } };
    const temp = body.current?.temperature_2m;
    if (typeof temp !== "number") return none();
    return Response.json({ city, temp: Math.round(temp), code: body.current?.weather_code ?? 0 }, { headers: { "Cache-Control": "private, max-age=1800" } });
  } catch {
    return none();
  }
}
