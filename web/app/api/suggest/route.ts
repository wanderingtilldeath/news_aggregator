import { getConcepts, getGraph } from "@/lib/api";
import { MAX_QUERY } from "@/lib/view";

// Search suggestions: names of the people, organisations and places in the news and glossary terms, whichever
// have a word that starts with what the reader has typed (a name that starts with it first). Source names are left
// out on purpose: they are labels ("Funding: Inc42"), not words the search finds. Both lists come from cached API
// reads, so a request per keystroke costs the origin nothing.
// shortcut: no prefix search over headlines (the search index matches whole words only), upgrade if names feel thin.
export async function GET(req: Request) {
  const q = (new URL(req.url).searchParams.get("q") ?? "").trim().slice(0, MAX_QUERY).toLowerCase();
  const headers = { "Cache-Control": "public, max-age=300" };
  if (q.length < 2) return Response.json({ suggestions: [] }, { headers });

  const [graph, concepts] = await Promise.all([getGraph(), getConcepts()]);
  const names = [
    ...(graph?.entities ?? []).map((e) => e.name),
    ...concepts.map((c) => c.term),
  ];

  const seen = new Set<string>();
  const starts: string[] = [];
  const contains: string[] = [];
  for (const name of names) {
    const key = name.toLowerCase();
    // A word start, not any substring: "tr" should find "Trump", not "Australia".
    const at = key.startsWith(q) ? 0 : key.search(new RegExp(`[\\s(/-]${q.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}`));
    if (at < 0 || seen.has(key)) continue;
    seen.add(key);
    (at === 0 ? starts : contains).push(name);
  }
  return Response.json({ suggestions: [...starts, ...contains].slice(0, 8) }, { headers });
}
