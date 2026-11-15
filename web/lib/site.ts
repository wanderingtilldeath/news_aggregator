// The public address of the site, for share links, the sitemap and the feed. Set SITE_URL where it
// differs from the default (a preview deployment, say).
// Where publishers and readers write about a takedown or a correction. Set CONTACT_EMAIL to a
// mailbox that reaches you.
export const CONTACT_EMAIL = process.env.CONTACT_EMAIL ?? "digest@postpaper.in";

// Pro: the Razorpay payment link readers pay through, and how its price reads ("Rs 99 for 31 days").
// Without the link the Pro page says Pro is opening soon.
export const PRO_CHECKOUT_URL = process.env.PRO_CHECKOUT_URL ?? "";
export const PRO_PRICE_TEXT = process.env.PRO_PRICE_TEXT ?? "";

export const SITE_URL = (process.env.SITE_URL ?? "https://www.postpaper.in").replace(/\/+$/, "");
