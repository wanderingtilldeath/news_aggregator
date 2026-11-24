// Run with: npm test (plain Node, no framework).
import assert from "node:assert/strict";
import { formatAmount, matches, parseRound, regionOf, rounds, stageGroup, totals } from "./funding.ts";

const story = (id: string, title: string, source = "entrackr.com", extra: { why?: string; summary?: string[] } = {}) => ({ id, title, source, time: "2026-11-18T08:00:00Z", ...extra });

// The headlines seen in the sources: company first, investor first, descriptor before the company.
let r = parseRound(story("1", "Botsync raises $11 Mn to connect robots and machines on factory floors"));
assert.deepEqual([r?.company, r?.amount, r?.stage, r?.investors], ["Botsync", { millions: 11, currency: "USD" }, "", []]);

r = parseRound(story("2", "Soket AI raises $4 Mn from Bajaj Finserv to expand its AI model horizons"));
assert.deepEqual([r?.company, r?.amount?.millions, r?.investors], ["Soket AI", 4, ["Bajaj Finserv"]]);

r = parseRound(story("3", "Physical AI startup Humyn Labs raises $47 Mn in equity and debt"));
assert.deepEqual([r?.company, r?.amount?.millions, r?.stage], ["Humyn Labs", 47, ""]); // "equity and debt" is not a round name

r = parseRound(story("4", "Felicis leads $7.5M bet on Unsiloed AI to make enterprise data usable", "techcrunch.com"));
assert.deepEqual([r?.company, r?.amount?.millions, r?.investors], ["Unsiloed AI", 7.5, ["Felicis"]]);

r = parseRound(story("5", "Fintech startup Zeta bags Rs 1,905 Cr in Series C led by Sequoia and Accel | Inc42"));
assert.deepEqual([r?.company, r?.amount, r?.stage, r?.investors], ["Zeta", { millions: 19050, currency: "INR" }, "Series C", ["Sequoia", "Accel"]]);

// Labels, backers and location leads are not part of the company name.
for (const [title, company] of [
  ["Exclusive: Zepto raises $350 Mn", "Zepto"],
  ["Funding Roundup: Zepto raises $5 Mn", "Zepto"],
  ["Sequoia-backed Pocket FM secures $103 Mn in Series D", "Pocket FM"],
  ["Bengaluru-based EV charging network operator ChargeZone raises $30 Mn", "ChargeZone"],
  ["Meesho-backed logistics startup Valmo raises $20 Mn", "Valmo"],
] as const) assert.equal(parseRound(story("n", title))?.company, company, title);

assert.equal(parseRound(story("6", "Acme secures Pre-Seed funding"))?.stage, "Pre-Seed");
assert.equal(parseRound(story("7", "Acme raises $2 million in seed round"))?.stage, "Seed");

// Not rounds: a valuation, a stake sale, an acquisition, a report.
for (const title of [
  "AI agent startup LangChain hits $3B valuation",
  "Ribbit Capital sells Rs 1,905 Cr stake in Groww through block deal",
  "Acme acquires Beta for $50 Mn",
  "Q3 2026 global startup funding hit $97B as AI rounds grow",
]) assert.equal(parseRound(story("x", title)), null, title);

// Money: a number with no unit, or a valuation, is not an amount.
assert.equal(parseRound(story("8", "Acme raises $5 from friends"))?.amount, null);
assert.equal(parseRound(story("9", "Acme raises funds at a $3 Bn valuation"))?.amount, null);

// The same round from several outlets is one row with every source; a round with no company stays alone.
const merged = rounds([
  story("a", "Botsync raises $11 Mn to connect robots", "entrackr.com"),
  story("b", "Botsync raises $11 Mn in Series A", "inc42.com"),
  story("c", "Raises $11 Mn to build robots", "techcrunch.com"),
  story("d", "Beta raises $11 Mn to do things", "inc42.com"),
  story("e", "Ribbit Capital sells stake in Groww", "entrackr.com"),
]);
assert.equal(merged.rounds.length, 3);
const botsync = merged.rounds.find((x) => x.company === "Botsync");
assert.deepEqual([botsync?.stories.length, botsync?.stage], [2, "Series A"]);
assert.equal(merged.other.length, 1);

// Display.
assert.equal(formatAmount({ millions: 11, currency: "USD" }), "$11 Mn");
assert.equal(formatAmount({ millions: 7.5, currency: "USD" }), "$7.5 Mn");
assert.equal(formatAmount({ millions: 1200, currency: "USD" }), "$1.2 Bn");
assert.equal(formatAmount({ millions: 19050, currency: "INR" }), "₹1,905 Cr");
assert.deepEqual(totals(merged.rounds), { USD: 33 });

// Filters.
assert.deepEqual(["Pre-Seed", "Series A", "Series C+", "Growth", "Venture Debt", ""].map(stageGroup), ["Seed", "Series A", "Series B+", "Growth", "Other", ""]);
const rs = merged.rounds;
assert.equal(regionOf({ ...rs[0], stories: [{ id: "x", source: "entrackr.com", title: "" }] }), "india");
assert.equal(regionOf({ ...rs[0], stories: [{ id: "x", source: "techcrunch.com", title: "" }], amount: { millions: 5, currency: "USD" } }), "global");
assert.equal(regionOf({ ...rs[0], stories: [{ id: "x", source: "techcrunch.com", title: "" }], amount: { millions: 500, currency: "INR" } }), "india");
assert.ok(matches(botsync!, "botsync") && matches(botsync!, "") && !matches(botsync!, "zeta"));

console.log("funding: all checks passed");
