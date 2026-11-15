import type { Metadata } from "next";
import { Suspense } from "react";
import { connection } from "next/server";
import PrintButton from "@/components/print-button";
import { getDigest, getEvents, getIntro, imageSrc, type Item } from "@/lib/api";
import { TRACKS, TRACK_IDS, catName, cleanTitle, hostName, score } from "@/lib/view";

export const metadata: Metadata = {
  title: "E-paper · Postpaper",
  description: "Today's Postpaper laid out like a newspaper, ready to print or save as a PDF.",
};

const dateLine = (ms: number) => new Date(ms).toLocaleDateString("en-IN", { weekday: "long", day: "numeric", month: "long", year: "numeric", timeZone: "Asia/Kolkata" });

function Brief({ it }: { it: Item }) {
  const point = (it.summary ?? []).find((b) => b && !/(…|\.\.\.)$/.test(b));
  return (
    <article className="ep-story">
      <h3>{cleanTitle(it.title || it.url)}</h3>
      {it.why ? (
        <p>
          <b>Why it matters:</b> {it.why}
        </p>
      ) : (
        point && <p>{point}</p>
      )}
      <p className="ep-by">
        {hostName(it.source)} · {catName(it.category || "other")}
      </p>
    </article>
  );
}

// Today's edition laid out like a newspaper page: a masthead and dateline, the lead story with its
// picture, and the rest in three columns. It is plain HTML and CSS, and the browser's print dialog
// ("Save as PDF") makes the file, so it costs the server nothing.
async function Paper() {
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();
  const [body, events, ...intros] = await Promise.all([getDigest({ days: "2", pageSize: 60 }).catch(() => null), getEvents(), ...TRACK_IDS.map((t) => getIntro(t).catch(() => null))]);
  if (!body?.items.length) return <p className="empty">Today&rsquo;s edition is not ready yet.</p>;

  const ranked = [...body.items].sort((a, b) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time));
  const lead = ranked.find((i) => i.image) ?? ranked[0];
  const rest = ranked.filter((i) => i !== lead).slice(0, 27);
  const picture = imageSrc(lead);
  const leadText = (lead.summary ?? []).filter((b) => b && !/(…|\.\.\.)$/.test(b)).slice(0, 3);

  return (
    <>
      <PrintButton />
      <header className="ep-head">
        <h1>PostPaper</h1>
        <p className="ep-dateline">
          <span>{dateLine(now)}</span>
          <span>The finished daily newspaper</span>
        </p>
      </header>

      <section className="ep-lead" aria-label="Lead story">
        <div>
          <h2>{cleanTitle(lead.title || lead.url)}</h2>
          {lead.why && (
            <p className="ep-why">
              <b>Why it matters:</b> {lead.why}
            </p>
          )}
          {leadText.length > 0 && (
            <ul>
              {leadText.map((b, i) => (
                <li key={i}>{b}</li>
              ))}
            </ul>
          )}
          <p className="ep-by">
            {hostName(lead.source)} · {catName(lead.category || "other")}
          </p>
        </div>
        {picture && (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={picture} alt="" referrerPolicy="no-referrer" />
        )}
      </section>

      {intros.some(Boolean) && (
        <section className="ep-brief" aria-label="Today in 60 seconds">
          <h2>Today in 60 seconds</h2>
          {intros.map((intro, i) => intro && <p key={TRACK_IDS[i]}><b>{TRACKS[TRACK_IDS[i]].title}.</b> {intro.text}</p>)}
        </section>
      )}

      {events.length > 0 && (
        <section className="ep-brief" aria-label="Coming up">
          <h2>
            Coming up <a href="/upcoming">See all →</a>
          </h2>
          {events.slice(0, 5).map((e) => (
            <p key={`${e.story}-${e.text}`}>
              <b>{new Date(`${e.day}T12:00:00Z`).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "UTC" })}.</b> {e.text}
            </p>
          ))}
        </section>
      )}

      <section className="ep-cols" aria-label="More stories">
        {rest.map((it) => (
          <Brief key={it.id} it={it} />
        ))}
      </section>
      <p className="fine-print">Summaries are written by an AI model from the articles; every story links to its publisher on postpaper.in.</p>
    </>
  );
}

export default function Page() {
  return (
    <main className="epaper" id="view">
      <Suspense fallback={null}>
        <Paper />
      </Suspense>
    </main>
  );
}
