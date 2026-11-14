// Kept apart from lib/api.ts, which holds server-only cached fetches, so client components can use it.

// Only absolute http(s) URLs reach href/src, so a hostile page cannot smuggle in a
// javascript: link. Relative URLs are resolved against the API (used for /img/{id}).
export function safeURL(u: string | undefined, base?: string): string {
  if (!u) return "";
  try {
    const x = new URL(u, base);
    return x.protocol === "http:" || x.protocol === "https:" ? x.href : "";
  } catch {
    return "";
  }
}

