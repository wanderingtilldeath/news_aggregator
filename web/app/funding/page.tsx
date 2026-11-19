import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import { Row } from "@/components/article";
import { getDigest } from "@/lib/api";
import { formatAmount, rounds, totals, type Currency } from "@/lib/funding";
import { ago, hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Startup funding · Postpaper",
  description: "Startup funding rounds of the last week, read from the headlines of Entrackr, Inc42, TechCrunch, Crunchbase News and Mint.",
};

const storyHref = (id: string) => `/story/${encodeURIComponent(id)}`;

// The funding rounds of the last seven days, read from the headlines of the "funding" sources (see lib/funding.ts).
// It reads the same cached digest call as every section page, so it adds no new API shape. A story that is not a
// round (an acquisition, a valuation, a market report) is listed under the table as startup news.
async function Body({ stage }: { stage: string }) {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const body = await getDigest({ days: "7", category: "funding", pageSize: 200 }).catch(() => null);
  if (!body) return <p className="empty">The funding news is not available right now. Please try again in a minute.</p>;
  if (!body.items.length) return <p className="empty">No funding news yet. It is collected a few times a day.</p>;

  const { rounds: all, other } = rounds(body.items);
  const stages = [...new Set(all.map((r) => r.stage).filter(Boolean))].sort();
  const shown = stage ? all.filter((r) => r.stage.toLowerCase() === stage.toLowerCase()) : all;
  const sums = Object.entries(totals(shown)) as [Currency, number][];
  const biggest = shown.filter((r) => r.amount && r.amount.currency === "USD").sort((a, b) => b.amount!.millions - a.amount!.millions)[0];

  return (
    <>
      <dl className="fund-stats">
        <div>
          <dt>Rounds</dt>
          <dd>{shown.length}</dd>
        </div>
        {sums.map(([currency, millions]) => (
          <div key={currency}>
            <dt>Disclosed in {currency}</dt>
            <dd>{formatAmount({ millions, currency })}</dd>
          </div>
        ))}
        {biggest?.amount && (
          <div>
            <dt>Largest in USD</dt>
            <dd>
              {formatAmount(biggest.amount)} <span>{biggest.company}</span>
            </dd>
          </div>
        )}
      </dl>

      {stages.length > 0 && (
        <nav className="subnav" aria-label="Round">
          <Link href="/funding" aria-current={stage ? undefined : "true"}>
            All
          </Link>
          {stages.map((s) => (
            <Link key={s} href={`/funding?stage=${encodeURIComponent(s)}`} aria-current={s.toLowerCase() === stage.toLowerCase() ? "true" : undefined}>
              {s}
            </Link>
          ))}
        </nav>
      )}

      {shown.length > 0 ? (
        <div className="fund-table" role="region" aria-label="Funding rounds" tabIndex={0}>
          <table>
            <thead>
              <tr>
                <th scope="col">Company</th>
                <th scope="col">Round</th>
                <th scope="col">Amount</th>
                <th scope="col">Investors</th>
                <th scope="col">Reported by</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => (
                <tr key={r.stories[0].id}>
                  <th scope="row">
                    <Link href={storyHref(r.stories[0].id)}>{r.company || "See the story"}</Link>
                    <time dateTime={r.time}>{ago(r.time, now)}</time>
                  </th>
                  <td>{r.stage || "—"}</td>
                  <td>{r.amount ? formatAmount(r.amount) : "—"}</td>
                  <td>{r.investors.length ? r.investors.join(", ") : "—"}</td>
                  <td>
                    {r.stories.map((s, i) => (
                      <span key={s.id}>
                        {i > 0 && ", "}
                        <Link href={storyHref(s.id)}>{hostName(s.source)}</Link>
                      </span>
                    ))}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <p className="empty">No {stage} rounds in the last week.</p>
      )}

      {!stage && other.length > 0 && (
        <section aria-label="More startup news">
          <div className="sec-head">
            <h2>More startup news</h2>
          </div>
          <div className="results">
            {other.slice(0, 12).map((it) => (
              <Row key={it.id} it={it} now={now} />
            ))}
          </div>
        </section>
      )}
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/funding">) {
  return (
    <main className="page wide-page" id="view">
      <h1>Startup funding</h1>
      <p className="lead-in">Funding rounds of the last week, read from the headlines of Entrackr, Inc42, TechCrunch, Crunchbase News and Mint. A round several outlets reported is one row.</p>
      <p className="fine-print">
        The company, amount, round and investors are read from headlines by simple rules, so a blank means the headline did not say, and a figure can be wrong. Check the story before you rely on it.
        Amounts are shown as reported, in dollars or rupees, and are not converted.
      </p>
      <Suspense fallback={<Loading />}>
        <Params searchParams={searchParams} />
      </Suspense>
      <p>
        <Link href="/">Back to the news</Link>
      </p>
    </main>
  );
}

async function Params({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const sp = await searchParams;
  return <Body stage={typeof sp.stage === "string" ? sp.stage.slice(0, 20) : ""} />;
}
