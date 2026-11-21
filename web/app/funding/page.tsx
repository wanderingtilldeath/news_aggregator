import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import { connection } from "next/server";
import Loading from "@/components/loading";
import { Card } from "@/components/article";
import SectionFront from "@/components/section-front";
import { getDigest, type Item } from "@/lib/api";
import { STAGE_GROUPS, formatAmount, matches, regionOf, rounds, stageGroup, totals, type Currency } from "@/lib/funding";
import { ago, hostName } from "@/lib/view";

export const metadata: Metadata = {
  title: "Startup funding · Postpaper",
  description: "Startup funding rounds of the last week, read from the headlines of Entrackr, Inc42, TechCrunch, Crunchbase News and Mint.",
};

const storyHref = (id: string) => `/story/${encodeURIComponent(id)}`;

type Filters = { stage: string; region: string; q: string };

// The page's own URL for a filter state, so the pills, the toggle and the search form all keep each other's choices.
function href(f: Filters, change: Partial<Filters>) {
  const n = { ...f, ...change };
  const sp = new URLSearchParams();
  if (n.stage) sp.set("stage", n.stage);
  if (n.region) sp.set("region", n.region);
  if (n.q) sp.set("q", n.q);
  const qs = sp.toString();
  return qs ? `/funding?${qs}` : "/funding";
}

// The funding rounds of the last seven days, read from the headlines of the "funding" sources (see lib/funding.ts).
// It reads the same cached digest call as every section page, so it adds no new API shape. A story that is not a
// round (an acquisition, a valuation, a market report) is listed under the table as startup news.
async function Body({ f }: { f: Filters }) {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const body = await getDigest({ days: "7", category: "funding", pageSize: 200 }).catch(() => null);
  if (!body) return <p className="empty">The funding news is not available right now. Please try again in a minute.</p>;
  if (!body.items.length) return <p className="empty">No funding news yet. It is collected a few times a day.</p>;

  const { rounds: all, other } = rounds(body.items);
  // The pill counts follow the search and the region, but not the stage itself, so each pill says what it would show.
  const scoped = all.filter((r) => matches(r, f.q) && (!f.region || regionOf(r) === f.region));
  const counts = (g: string) => scoped.filter((r) => stageGroup(r.stage) === g).length;
  const shown = f.stage ? scoped.filter((r) => stageGroup(r.stage) === f.stage) : scoped;
  const filtered = Boolean(f.stage || f.region || f.q);
  const sums = Object.entries(totals(shown)) as [Currency, number][];
  const biggest = shown.filter((r) => r.amount && r.amount.currency === "USD").sort((a, b) => b.amount!.millions - a.amount!.millions)[0];

  // The front: the biggest dollar round of the week leads, then the newest rounds that name a company and an amount,
  // each with a line (company, amount, round) under its headline. It is shown only when nothing is filtered.
  const byId = new Map(body.items.map((it) => [it.id, it]));
  const named = all.filter((r) => r.company && r.amount);
  const lead = named.filter((r) => r.amount!.currency === "USD").sort((a, b) => b.amount!.millions - a.amount!.millions)[0];
  const ordered = lead ? [lead, ...named.filter((r) => r !== lead)] : named;
  const frontItems = ordered.map((r) => byId.get(r.stories[0].id)).filter((it): it is Item => Boolean(it));
  const tags = Object.fromEntries(ordered.map((r) => [r.stories[0].id, [r.company, formatAmount(r.amount!), r.stage].filter(Boolean).join(" · ")]));

  return (
    <>
      <dl className="fund-stats">
        <div>
          <dt>Rounds this week</dt>
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

      {!filtered && frontItems.length >= 6 && (
        <SectionFront items={frontItems} label="Startups" now={now} tags={tags} leadKicker={lead ? "Biggest round of the week" : "Funding"} listTitle="More rounds" />
      )}

      <div className="fund-filters">
        <form action="/funding" method="get" role="search" className="fund-search">
          {f.stage && <input type="hidden" name="stage" value={f.stage} />}
          {f.region && <input type="hidden" name="region" value={f.region} />}
          <input type="search" name="q" defaultValue={f.q} placeholder="Search a company or investor" aria-label="Search a company or investor" maxLength={60} />
          <button type="submit">Search</button>
        </form>
        <nav className="seg" aria-label="Region">
          <Link href={href(f, { region: "" })} aria-current={f.region ? undefined : "true"}>
            All
          </Link>
          <Link href={href(f, { region: "india" })} aria-current={f.region === "india" ? "true" : undefined}>
            India
          </Link>
          <Link href={href(f, { region: "global" })} aria-current={f.region === "global" ? "true" : undefined}>
            Global
          </Link>
        </nav>
      </div>

      <nav className="subnav fund-pills" aria-label="Round">
        <Link href={href(f, { stage: "" })} aria-current={f.stage ? undefined : "true"}>
          All <span className="n">{scoped.length}</span>
        </Link>
        {STAGE_GROUPS.filter((g) => counts(g) > 0 || g === f.stage).map((g) => (
          <Link key={g} href={href(f, { stage: g })} aria-current={g === f.stage ? "true" : undefined}>
            {g} <span className="n">{counts(g)}</span>
          </Link>
        ))}
      </nav>

      {shown.length > 0 ? (
        <>
          <div className="sec-head fund-head">
            <h2>All funding rounds</h2>
            <span className="sub">{shown.length} in the last 7 days</span>
          </div>
          <div className="fund-table" role="region" aria-label="Funding rounds">
            <table>
              <thead>
                <tr>
                  <th scope="col">Company</th>
                  <th scope="col">Investors</th>
                  <th scope="col">Reported by</th>
                  <th scope="col" className="amt">Amount</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((r) => (
                  <tr key={r.stories[0].id}>
                    <th scope="row">
                      <Link href={storyHref(r.stories[0].id)}>{r.company || "See the story"}</Link>
                      <span className="meta">
                        {r.stage && <span className="tag stage">{r.stage}</span>}
                        <time dateTime={r.time}>{ago(r.time, now)}</time>
                      </span>
                    </th>
                    <td data-label="Investors">
                      {r.investors.length ? r.investors.map((v) => <span key={v} className="tag">{v}</span>) : <span className="undisclosed">—</span>}
                    </td>
                    <td data-label="Reported by">
                      {r.stories.map((s, i) => (
                        <span key={s.id}>
                          {i > 0 && ", "}
                          <Link href={storyHref(s.id)}>{hostName(s.source)}</Link>
                        </span>
                      ))}
                    </td>
                    <td className="amt" data-label="Amount">
                      {r.amount ? formatAmount(r.amount) : <span className="undisclosed">Undisclosed</span>}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <p className="empty">
          No rounds match{filtered ? " these filters" : ""} in the last week. {filtered && <Link href="/funding">Clear the filters</Link>}
        </p>
      )}

      {!filtered && other.length > 0 && (
        <section aria-label="More startup news">
          <div className="sec-head">
            <h2>More startup news</h2>
          </div>
          <div className="cards">
            {other.slice(0, 12).map((it) => (
              <Card key={it.id} it={it} now={now} />
            ))}
          </div>
        </section>
      )}
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/funding">) {
  return (
    <main className="content" id="view">
      <div className="sectionhead">
        <h1>Startups</h1>
      </div>
      <Suspense fallback={<Loading />}>
        <Params searchParams={searchParams} />
      </Suspense>
    </main>
  );
}

async function Params({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const sp = await searchParams;
  const one = (k: string, max: number) => (typeof sp[k] === "string" ? (sp[k] as string).trim().slice(0, max) : "");
  const stage = one("stage", 20);
  const region = one("region", 10);
  return <Body f={{ stage: (STAGE_GROUPS as readonly string[]).includes(stage) ? stage : "", region: region === "india" || region === "global" ? region : "", q: one("q", 60) }} />;
}
