import type { Metadata } from "next";
import Link from "next/link";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { connection } from "next/server";
import Logo from "@/components/logo";
import PrintButton from "@/components/print-button";
import { getDigest, getIntro, getProStatus, getQuiz, type Item } from "@/lib/api";
import { SITE_URL } from "@/lib/site";
import { TRACKS, TRACK_IDS, catName, cleanTitle, hostName, score } from "@/lib/view";

export const metadata: Metadata = { title: "Printable edition · Postpaper Pro", robots: { index: false, follow: false } };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v) ?? "";

// The headings of the sections, as in the daily email: the kind of news, never an exam or a profession.
const SECTION_TITLES: Record<string, string> = {
  india: "India",
  world: "World",
  tech: "Technology",
  ai: "AI",
  funding: "Startups and funding",
  editorial: "Editorials",
  upsc: "Policy and governance",
};
const SECTION_ORDER = Object.keys(SECTION_TITLES);

const byImportance = (a: Item, b: Item) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time);

const storyHref = (it: Item) => `${SITE_URL}/story/${encodeURIComponent(it.id)}`;

// One story. Its headline and its last line both open the story's page on Postpaper (which links on to the
// publisher), never the publisher's own address.
function Story({ it, lead }: { it: Item; lead?: boolean }) {
  const bullets = (it.summary ?? []).filter((b) => b && !/(…|\.\.\.)$/.test(b)).slice(0, lead ? 3 : 2);
  const href = storyHref(it);
  return (
    <li className={lead ? "issue-story lead" : "issue-story"}>
      <p className="issue-cat">
        {SECTION_TITLES[it.category || ""] ?? catName(it.category || "other")} · {hostName(it.source)}
      </p>
      <h3>
        <a href={href}>{cleanTitle(it.title || it.url)}</a>
      </h3>
      {it.why && (
        <p className="issue-why">
          <b>Why it matters</b> {it.why}
        </p>
      )}
      {lead && it.long ? (
        <p>{it.long}</p>
      ) : (
        bullets.length > 0 && (
          <ul>
            {bullets.map((b, i) => (
              <li key={i}>{b}</li>
            ))}
          </ul>
        )
      )}
      <p className="issue-link">
        <a href={href}>Read the full story on Postpaper →</a>
      </p>
    </li>
  );
}

// One edition for printing, laid out like an issue: a nameplate and date, "Today in 60 seconds", the top stories,
// a section for each kind of news, and the quiz with its answers. ?day= picks an edition day and ?scope=week the
// last seven days (stories only). The browser's print dialog ("Save as PDF") makes the file.
async function Edition({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  await connection();
  const sp = await searchParams;
  const token = first(sp.token);
  const status = token ? await getProStatus(token) : null;

  if (status === "error") return <p className="empty">We could not check your link just now. Please try again in a minute.</p>;
  if (!status) {
    return (
      <p className="empty">
        This link is not valid. Use the link in your latest Pro email, or <Link href="/pro">see Postpaper Pro</Link>.
      </p>
    );
  }
  if (!status.active) {
    return (
      <p className="empty">
        Your Pro membership ended on {status.until}. <Link href="/pro">Renew it</Link> to open the printable editions again.
      </p>
    );
  }

  const week = first(sp.scope) === "week";
  const day = /^\d{4}-\d{2}-\d{2}$/.test(first(sp.day)) ? first(sp.day) : undefined;
  const cats = [...new Set([...TRACK_IDS.flatMap((t) => TRACKS[t].categories), ...SECTION_ORDER])];
  const pages = await Promise.all(
    cats.map((c) => getDigest({ days: week ? "7" : day ? "1" : "2", day: week ? undefined : day, category: c, pageSize: 60, detail: true }).catch(() => null)),
  );
  const all = pages.flatMap((p) => p?.items ?? []);
  if (!all.length) return <p className="empty">Nothing in this edition yet.</p>;

  // The most important stories across everything lead; each section then adds the best of what is left.
  const top = [...all].sort(byImportance).slice(0, week ? 10 : 8);
  const used = new Set(top.map((it) => it.id));
  const sections = SECTION_ORDER.map((cat) => ({
    cat,
    items: all
      .filter((it) => (it.category || "other") === cat && !used.has(it.id))
      .sort(byImportance)
      .slice(0, week ? 6 : 4),
  })).filter((s) => s.items.length > 0);

  // The briefing is the general edition's, or the two track briefings joined when it is not written yet.
  let intro = "";
  if (!week) {
    const general = await getIntro("top", day).catch(() => null);
    intro = general?.text ?? (await Promise.all(TRACK_IDS.map((t) => getIntro(t, day).catch(() => null)))).map((i) => i?.text).filter(Boolean).join(" ");
  }
  const quizzes = week ? [] : (await Promise.all(TRACK_IDS.map((t) => getQuiz(t, day).catch(() => null)))).filter((q) => q && q.questions.length > 0);

  const when = day ? new Date(`${day}T12:00:00+05:30`) : new Date();
  const date = when.toLocaleDateString("en-IN", { weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "Asia/Kolkata" });
  const count = top.length + sections.reduce((n, s) => n + s.items.length, 0);

  return (
    <>
      <PrintButton />
      <header className="issue-head">
        <Logo size="lg" />
        <p className="issue-kicker">Pro edition</p>
        <h1>{week ? "The week in review" : "The daily edition"}</h1>
        <p className="issue-date">
          {date} · {count} stories
        </p>
      </header>

      {intro && (
        <aside className="issue-intro">
          <b>Today in 60 seconds</b>
          <p>{intro}</p>
        </aside>
      )}

      <section>
        <h2 className="issue-sec">Top stories</h2>
        <ol className="issue-stories">
          {top.map((it, i) => (
            <Story key={it.id} it={it} lead={i < 2} />
          ))}
        </ol>
      </section>

      {sections.map((s) => (
        <section key={s.cat}>
          <h2 className="issue-sec">{SECTION_TITLES[s.cat]}</h2>
          <ol className="issue-stories">
            {s.items.map((it) => (
              <Story key={it.id} it={it} />
            ))}
          </ol>
        </section>
      ))}

      {quizzes.length > 0 && (
        <section className="issue-quiz">
          <h2 className="issue-sec">Quiz, with answers</h2>
          <ol>
            {quizzes.flatMap((q) =>
              q!.questions.map((qq, i) => (
                <li key={`${q!.track}${i}`}>
                  {qq.question}
                  <ul>
                    {qq.options.map((o, j) => (
                      <li key={j} className={j === qq.answer ? "right" : undefined}>
                        {String.fromCharCode(65 + j)}. {o}
                        {j === qq.answer ? " ✓" : ""}
                      </li>
                    ))}
                  </ul>
                  <small>{qq.explanation}</small>
                </li>
              )),
            )}
          </ol>
        </section>
      )}

      <footer className="issue-foot">
        Every headline opens the full story on Postpaper, which links on to the publisher. · {SITE_URL.replace(/^https?:\/\//, "")}
      </footer>
    </>
  );
}

export default function Page({ searchParams }: PageProps<"/pro/edition">) {
  return (
    <main className="issue" id="view">
      <Suspense fallback={<Loading />}>
        <Edition searchParams={searchParams} />
      </Suspense>
    </main>
  );
}
