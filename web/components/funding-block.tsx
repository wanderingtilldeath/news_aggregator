import Link from "next/link";
import { getDigest } from "@/lib/api";
import { formatAmount, rounds } from "@/lib/funding";
import { ago, hostName } from "@/lib/view";

// A short "this week" strip on the front page that leads to /funding. It reads the same cached digest call as
// that page and shows the newest rounds that name a company and an amount; with none (a quiet week, the API down)
// it renders nothing.
export default async function FundingBlock({ now }: { now: number }) {
  const body = await getDigest({ days: "7", category: "funding", pageSize: 200 }).catch(() => null);
  const top = rounds(body?.items ?? []).rounds.filter((r) => r.company && r.amount).slice(0, 5);
  if (!top.length) return null;

  return (
    <section className="fund-block" aria-label="Startup funding this week">
      <div className="sec-head">
        <h2>Startup funding this week</h2>
        <Link href="/funding">All rounds →</Link>
      </div>
      <ol>
        {top.map((r) => (
          <li key={r.stories[0].id}>
            <b className="amt">{formatAmount(r.amount!)}</b>
            <Link href={`/story/${encodeURIComponent(r.stories[0].id)}`}>{r.company}</Link>
            {r.stage && <span className="tag stage">{r.stage}</span>}
            <span className="by">
              {hostName(r.stories[0].source)}
              {r.stories.length > 1 ? ` +${r.stories.length - 1}` : ""} · {ago(r.time, now)}
            </span>
          </li>
        ))}
      </ol>
    </section>
  );
}
