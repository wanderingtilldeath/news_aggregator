// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { STOP, gazetteer, matchStories, mentions, type Place, type Story } from "./map-news.ts";
import type { MapLayer } from "./maps.ts";

const read = (f: string) => JSON.parse(readFileSync(new URL(f, import.meta.url), "utf8"));
const layers: MapLayer[] = readdirSync(new URL("./data/maps/", import.meta.url)).filter((f) => f.endsWith(".json") && f !== "index.json").map((f) => read(`./data/maps/${f}`));
const states = read("./data/india.json").states as { id: string; name: string }[];

// A name counts only as a whole word with its capital.
assert.ok(mentions("Floods hit Kerala's coast", "Kerala"));
assert.ok(mentions("Kerala-based firm", "Kerala"));
assert.ok(mentions("Talks in New Delhi", "Delhi"));
assert.ok(!mentions("A Keralan custom", "Kerala"));
assert.ok(!mentions("kerala floods", "Kerala"));
assert.ok(!mentions("Goan food", "Goa"));
assert.ok(mentions("Dr. (Mrs.) A.B. said", "A.B."), "a dot in a name is not a wildcard");
assert.ok(!mentions("AxB", "A.B"));

const places = gazetteer(layers, states);
const byName = (n: string) => places.find((p) => p.name === n)!;

// One place a name, and the names a story may use.
assert.equal(new Set(places.map((p) => p.key)).size, places.length, "keys are unique");
assert.equal(new Set(places.map((p) => p.name.toLowerCase())).size, places.length, "one place a name, however many maps list it");
assert.ok(places.every((p) => p.names.every((n) => (n.length >= 4 || n === "Goa" || n === "J&K") && !STOP.has(n))), "no short or ambiguous name");
assert.ok(places.every((p) => p.shape || p.d || (Number.isFinite(p.lon) && Number.isFinite(p.lat))), "every place can be drawn");
assert.ok(places.length > 400, `${places.length} places`);

// Mumbai is a city, a capital and a port: one place on three maps. Delhi and Goa are states, not city dots.
assert.ok(byName("Mumbai").on.length >= 3);
assert.ok(byName("Delhi").shape, "Delhi is the state shape");
assert.equal(places.filter((p) => p.name === "Goa").length, 1);
assert.ok(byName("Goa").shape);
// A river is a line, a dam is a point with and without "Dam" in its names.
assert.ok(byName("Godavari").d);
assert.deepEqual(byName("Tehri Dam").names.sort(), ["Tehri", "Tehri Dam"]);
assert.ok(byName("Ganga").names.includes("Ganges"));
assert.ok(byName("Bengaluru").names.includes("Bangalore"));
// Brackets are notes, not part of the name; river sources and the monsoon map are not places of their own.
assert.ok(places.every((p) => !p.name.includes("(")));
assert.ok(!places.some((p) => p.key.startsWith("rivers:") || p.key.startsWith("monsoon:")));
// A river named like a person needs the word "river"; a park is not shortened to a first name.
assert.deepEqual(byName("Krishna").names, ["Krishna River", "Krishna river", "River Krishna"]);
assert.ok(!places.some((p) => p.names.some((n) => ["Sanjay", "Madhav", "Desert", "Kali"].includes(n))));
assert.ok(byName("Desert National Park").names.includes("Desert National Park"));
// An airport is not shortened to a person's name.
assert.ok(!places.some((p) => p.names.includes("Indira Gandhi")));

// Matching: counts, newest first, the cap on what is kept, and the order of places.
const at = (h: number) => new Date(Date.UTC(2026, 10, 24, h)).toISOString();
const story = (id: string, title: string, h: number, extra: Partial<Story> = {}): Story => ({ id, title, source: "The Hindu", time: at(h), ...extra });
const stories = [
  story("g", "Lord Krishna festival crowds", 2),
  story("h", "Krishna river in spate near Vijayawada", 6),
  story("a", "Kerala floods displace thousands", 1),
  story("b", "Heavy rain lashes Kerala and Karnataka", 5),
  story("c", "Tehri Dam level rises after rain", 3),
  story("d", "Markets rally", 9, { why: "Matters for Bangalore start-ups", tags: ["Kerala"] }),
  story("e", "A keralan custom", 7),
  story("f", "Flood alert on the Ganges at Patna", 4),
];
const m = matchStories(places, stories, 2);
const find = (n: string) => m.find((x) => x.place.name === n);
assert.equal(find("Kerala")!.count, 3, "title, tag and title again; the lower-case 'keralan' story does not count");
assert.deepEqual(find("Kerala")!.stories.map((s) => s.id), ["d", "b"], "newest first, two kept");
assert.equal(m[0].place.name, "Kerala", "most stories first");
assert.equal(find("Karnataka")!.count, 1);
assert.deepEqual(find("Tehri Dam")!.stories.map((s) => s.id), ["c"]);
assert.deepEqual(find("Bengaluru")!.stories.map((s) => s.id), ["d"], "an older spelling in the summary counts");
assert.deepEqual(find("Ganga")!.stories.map((s) => s.id), ["f"]);
assert.ok(find("Patna"), "a capital in a title counts");
assert.ok(!find("Goa"), "a place no story names is left out");
assert.deepEqual(find("Krishna")!.stories.map((s) => s.id), ["h"], "the god is not the river");
assert.deepEqual(matchStories(places, []), []);

const sample: Place = { key: "x", name: "Test", on: [], names: ["Test"] };
assert.equal(matchStories([sample], [story("z", "Test", 1)])[0].count, 1);

console.log("map-news: ok");
