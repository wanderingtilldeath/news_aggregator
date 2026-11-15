// The public address of the site, for share links, the sitemap and the feed. Set SITE_URL where it
// differs from the default (a preview deployment, say).
// Where publishers and readers write about a takedown or a correction. Set CONTACT_EMAIL to a
// mailbox that reaches you.
export const CONTACT_EMAIL = process.env.CONTACT_EMAIL ?? "digest@postpaper.in";

// Pro: one Razorpay payment link per plan, and how each price reads. The amounts must match the
// server's PRO_PLANS (99 rupees buys 31 days, 249 rupees 92 days by default). A plan without a link
// is not offered; with neither the Pro page says Pro is opening soon.
export const PRO_PLANS = [
  { id: "monthly", url: process.env.PRO_MONTHLY_URL ?? "", price: process.env.PRO_MONTHLY_PRICE ?? "Rs 99 a month" },
  { id: "quarterly", url: process.env.PRO_QUARTERLY_URL ?? "", price: process.env.PRO_QUARTERLY_PRICE ?? "Rs 249 for 3 months" },
];

export const SITE_URL = (process.env.SITE_URL ?? "https://www.postpaper.in").replace(/\/+$/, "");
