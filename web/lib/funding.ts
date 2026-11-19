import type { Item } from "./api";

// The startup funding tracker reads funding rounds out of the headlines of the stories in the "funding"
// category ("Botsync raises $11 Mn from X to ..."), with plain rules: no model, no network, no storage.
// A headline names the company, the amount, often the round and the lead investor, and a rule that cannot
// find one leaves it blank rather than guessing. A story that is not a round (an acquisition, a valuation, a
// stake sale) returns null and is listed as news instead. This file has no server code and no imports other
// than a type, so it runs under plain Node for its tests (funding.test.ts).

export type Currency = "USD" | "INR" | "EUR" | "GBP";

export type Round = {
  company: string;
  stage: string; // "Seed", "Series A", ... or ""
  amount: { millions: number; currency: Currency } | null; // millions of the currency
  investors: string[];
  time: string;
  // The stories that reported this round: more than one outlet usually did.
  stories: { id: string; source: string; title: string }[];
};

const SYMBOLS: [RegExp, Currency][] = [
  [/^(?:US\$|\$|USD)$/i, "USD"],
  [/^(?:₹|Rs\.?|INR)$/i, "INR"],
  [/^(?:€|EUR)$/i, "EUR"],
  [/^(?:£|GBP)$/i, "GBP"],
];

// "$11 Mn", "$7.5M", "Rs 1,905 Cr", "USD 11 million", "€20 million".
const AMOUNT = /(US\$|\$|USD|₹|Rs\.?|INR|€|EUR|£|GBP)\s?(\d[\d,]*(?:\.\d+)?)\s?(billion|bn|b|million|mn|m|crore|cr|lakh|thousand|k)?(?![a-z])/i;

const RAISE = /\b(raises?|raised|bags?|bagged|secures?|secured|lands?|landed|gets|nabs|closes|closed|picks? up|mops? up|garners?|receives?|scores?|snags?|banks|pockets|infuses?|infused)\b/i;

const STAGE = /\b(pre-?seed|seed|pre-?series [a-k]|series [a-k]\+?|bridge|angel|growth|venture debt|grant)\b/i;

// Millions of the amount's currency: 1 crore is 10 million, 1 lakh is 0.1 million.
function millions(value: number, unit: string): number | null {
  switch (unit.toLowerCase()) {
    case "billion":
    case "bn":
    case "b":
      return value * 1000;
    case "million":
    case "mn":
    case "m":
      return value;
    case "crore":
    case "cr":
      return value * 10;
    case "lakh":
      return value / 10;
    case "thousand":
    case "k":
      return value / 1000;
    default:
      return null; // no unit: not an amount we can read
  }
}

function amountIn(text: string): Round["amount"] {
  const m = AMOUNT.exec(text);
  if (!m || !m[3]) return null;
  const currency = SYMBOLS.find(([re]) => re.test(m[1]))?.[1];
  const value = millions(parseFloat(m[2].replace(/,/g, "")), m[3]);
  // "a $3B valuation" or "valued at $3B" is not money raised.
  if (!currency || value === null || /^\s*(valuation|valued)/i.test(text.slice(m.index + m[0].length))) return null;
  return { millions: value, currency };
}

// The headline without a trailing " | Site" or " - Site" tail.
const tidy = (title: string) => title.replace(/\s+[|–—-]\s+[^|–—-]{2,40}$/, "").replace(/\s+/g, " ").trim();

// The company: the words before the verb, after a descriptor such as "AI agent startup".
function companyBefore(head: string): string {
  let name = head.replace(/^["'“‘]+|["'”’]+$/g, "").trim();
  const descriptor = /\b(?:startup|start-up|unicorn|firm|platform|company|maker|giant)\s+/gi;
  let last = 0;
  for (let m = descriptor.exec(name); m; m = descriptor.exec(name)) last = m.index + m[0].length;
  name = name.slice(last).replace(/^(the|a|an)\s+/i, "").replace(/[,:;]+$/, "").trim();
  return name.split(/\s+/).length > 6 || name.length > 60 ? "" : name;
}

const NOT_AN_INVESTOR = /^(?:series|seed|the|a|an|its|their|his|her|new|existing|equity|debt|funding|round)\b|^\d/i;

// Investors named after "from", "led by", "backed by" or "participation from", up to the next clause.
function investorsIn(text: string): string[] {
  const m = /\b(?:co-led by|led by|backed by|participation from|from)\s+([A-Z][^.;:()]*?)(?=\s+(?:to|for|at|in|as|with|which|that|and (?:will|plans|to))\b|[.;:(]|$)/.exec(text);
  if (!m) return [];
  const names = m[1]
    .split(/\s*(?:,|&|\band\b)\s*/)
    .map((n) => n.trim())
    .filter((n) => n && !NOT_AN_INVESTOR.test(n) && n.split(/\s+/).length <= 4);
  return [...new Set(names)].slice(0, 4);
}

// parseRound reads one story. It returns null when the headline is not a funding round.
export function parseRound(it: Pick<Item, "id" | "title" | "why" | "summary" | "source" | "time">): Round | null {
  const title = tidy(it.title || "");
  const detail = [it.why, ...(it.summary ?? [])].filter(Boolean).join(" ");

  let company = "";
  let investors: string[] = [];

  const verb = RAISE.exec(title);
  // "Felicis leads $7.5M bet on Unsiloed AI": the investor comes first.
  const led = /^(.+?)\s+(?:leads|co-leads|backs)\b(.*)$/i.exec(title);
  if (verb) {
    company = companyBefore(title.slice(0, verb.index));
  } else if (led) {
    investors = [led[1].trim()];
    const target = /\b(?:on|in|into)\s+(.+?)(?=\s+(?:to|as|for|and|with)\b|[,:–—]|$)/i.exec(led[2]);
    company = target ? companyBefore(target[1]) : "";
  } else {
    return null;
  }

  const stage = STAGE.exec(title)?.[1] ?? STAGE.exec(detail)?.[1] ?? "";
  const named = investors.length ? investors : investorsIn(title);

  return {
    company,
    stage: stage ? stage.replace(/\b\w/g, (c) => c.toUpperCase()).replace(/(Pre)-?(Seed|Series)/i, "$1-$2") : "",
    amount: amountIn(title) ?? amountIn(detail),
    investors: named.length ? named : investorsIn(detail),
    time: it.time,
    stories: [{ id: it.id, source: it.source, title }],
  };
}

const norm = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, "");

// rounds reads the stories and merges the ones that report the same round (the same company, amount and
// currency), since several outlets cover most rounds. Newest first. A round with no company is never merged.
export function rounds<T extends Pick<Item, "id" | "title" | "why" | "summary" | "source" | "time">>(items: T[]): { rounds: Round[]; other: T[] } {
  const merged = new Map<string, Round>();
  const solo: Round[] = [];
  const other: T[] = [];

  for (const it of items) {
    const r = parseRound(it);
    if (!r) {
      other.push(it);
      continue;
    }
    const key = r.company && r.amount ? `${norm(r.company)}|${r.amount.currency}${r.amount.millions}` : "";
    const have = key ? merged.get(key) : undefined;
    if (!have) {
      if (key) merged.set(key, r);
      else solo.push(r);
      continue;
    }
    have.stories.push(...r.stories);
    have.stage ||= r.stage;
    if (!have.investors.length) have.investors = r.investors;
    if (Date.parse(r.time) > Date.parse(have.time)) have.time = r.time;
  }

  const all = [...merged.values(), ...solo].sort((a, b) => Date.parse(b.time) - Date.parse(a.time));
  return { rounds: all, other };
}

// "$11 Mn", "$1.2 Bn", "₹1,905 Cr".
export function formatAmount(a: NonNullable<Round["amount"]>): string {
  const trim = (n: number) => (Number.isInteger(n) ? String(n) : n.toFixed(n < 10 ? 1 : 0).replace(/\.0$/, ""));
  if (a.currency === "INR") return `₹${Math.round(a.millions / 10).toLocaleString("en-IN")} Cr`;
  const symbol = { USD: "$", EUR: "€", GBP: "£" }[a.currency];
  return a.millions >= 1000 ? `${symbol}${trim(a.millions / 1000)} Bn` : `${symbol}${trim(a.millions)} Mn`;
}

// The money disclosed in these rounds, per currency (no conversion between currencies).
export function totals(list: Round[]): Partial<Record<Currency, number>> {
  const out: Partial<Record<Currency, number>> = {};
  for (const r of list) if (r.amount) out[r.amount.currency] = (out[r.amount.currency] ?? 0) + r.amount.millions;
  return out;
}
