// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { WEATHER_FRESH_MS, freshWeather, packWeather } from "./weather-cache.ts";

const now = 1_700_000_000_000;
const body = { city: "Bengaluru", temp: 28, code: 2 };

assert.deepEqual(freshWeather(packWeather(body, now - 60_000), now), body);
assert.equal(freshWeather(packWeather(body, now - WEATHER_FRESH_MS), now), null, "exactly half an hour is stale");
assert.equal(freshWeather(packWeather(body, now + 5_000), now), null, "a stored time in the future is not trusted");
assert.equal(freshWeather("", now), null);
assert.equal(freshWeather("not json", now), null);
assert.equal(freshWeather("null", now), null);
assert.equal(freshWeather(JSON.stringify({ at: now, body: {} }), now), null, "an answer with no city is not worth keeping");
assert.equal(freshWeather(JSON.stringify({ at: now, body: { city: "X", temp: "hot" } }), now), null);
assert.deepEqual(freshWeather(JSON.stringify({ at: now, body: { city: "Pune", temp: 30 } }), now), { city: "Pune", temp: 30, code: 0 });

console.log("weather-cache: ok");
