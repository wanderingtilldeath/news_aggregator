import Link from "next/link";
import { notFound } from "next/navigation";
import { connection } from "next/server";
import { Suspense } from "react";
import { Card, Meter, Signals, SiteTile, fullText } from "@/components/article";
import ExamLens from "@/components/exam-lens";
import ReadAloud from "@/components/read-aloud";
import SaveNote from "@/components/save-note";
import Share from "@/components/share";
import { getConcepts, getDigest, getStory, imageSrc, safeURL } from "@/lib/api";
import { conceptId, mentioned } from "@/lib/concepts";
import { entityHref } from "@/lib/graphlayout";
import { SOURCE_TYPES, TRACKS, ago, catName, cleanTitle, coverageByKind, hostName, trackOf } from "@/lib/view";

const shortDay = (iso?: string) => (iso ? new Date(iso).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "Asia/Kolkata" }) : "");

// One story on its own Postpaper page: the headline, our summary and the "why it matters"
// line, and a button that goes on to the publisher's article. The text is ours (a short
// summary), never the publisher's article, so the page points readers on rather than replacing it.
async function Story({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await connection();
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now();

  const it = await getStory(id);
  if (!it) notFound();

  const title = cleanTitle(it.title || it.url);
  const category = it.category || "other";
  const track = trackOf(category);
  const publisher = hostName(it.source);
  const external = safeURL(it.url);
  const image = imageSrc(it);
  const body = fullText(it);
  const bullets = (it.summary ?? []).filter(Boolean);
  // Bullets cut off by an older version of the summariser end in an ellipsis; they are not shown on
  // the story page, which opens with the article's own first sentences instead (the lead).
  const whole = bullets.filter((b) => !/(…|\.\.\.)$/.test(b));
  const readable = whole.length >= 2 ? whole : it.lead ? [it.lead] : whole.length === 1 ? whole : [];
  const blurb = (it.why || body).slice(0, 180);

  // More from the same section, from the same cached call the section page makes.
  const more = await getDigest({ days: "7", category, pageSize: 8 }).catch(() => null);
  const related = (more?.items ?? []).filter((x) => x.id !== it.id).slice(0, 3);
  // The glossary terms this story uses, matched in its own text (nothing is stored per story).
  const concepts = mentioned([it.title, it.why, ...(it.summary ?? []), it.lead].filter(Boolean).join(" "), await getConcepts());

  // The story so far: when the coverage of this event spans more than one day, lay it out oldest
  // first with this story marked; otherwise it stays a plain "also covered by" list.
  const days = new Set([it.time, ...(it.also ?? []).map((o) => o.time)].map((t) => (t ?? "").slice(0, 10)).filter(Boolean));
  const thread =
    days.size > 1
      ? [...(it.also ?? []), { id: it.id, title: it.title, source: it.source, time: it.time }].sort((a, b) => Date.parse(a.time ?? "") - Date.parse(b.time ?? ""))
      : null;

  return (
    <>
      {/* Rendered here, React moves these into <head>: a title and a share preview per story. */}
      <title>{`${title} · Postpaper`}</title>
      <meta name="description" content={blurb} />
      <meta property="og:type" content="article" />
      <meta property="og:title" content={title} />
      <meta property="og:description" content={blurb} />
      {image && <meta property="og:image" content={image} />}
      <meta name="twitter:card" content={image ? "summary_large_image" : "summary"} />

      <article className="story">
        <nav className="crumbs" aria-label="Breadcrumb">
          <Link href="/">Home</Link>
          <span aria-hidden="true">/</span>
          {track && (
            <>
              <Link href={`/?track=${track}`}>{TRACKS[track].title}</Link>
              <span aria-hidden="true">/</span>
            </>
          )}
          <Link href={`/?category=${encodeURIComponent(category)}`}>{catName(category)}</Link>
        </nav>

        <h1>{title}</h1>
        <div className="byline">
          <span>
            <b>{publisher}</b>
            {it.source_type && SOURCE_TYPES[it.source_type] && <span className="stype">{SOURCE_TYPES[it.source_type]}</span>} · {ago(it.time, now)}
          </span>
          <span className="signals">
            <Signals it={it} />
          </span>
          <Meter value={it.importance} />
        </div>

        {/* The picture sits over the site's tile, like on the cards: no image, or one that fails to
            load, leaves the tile instead of an empty space. */}
        <div className="media story-hero">
          <SiteTile source={it.source} />
          {image && (
            // eslint-disable-next-line @next/next/no-img-element
            <img className="img" src={image} alt="" referrerPolicy="no-referrer" />
          )}
        </div>

        {it.why && (
          <aside className="callout">
            <span className="eyebrow">Why it matters</span>
            <p>{it.why}</p>
          </aside>
        )}

        <section aria-label="Summary" className="summary">
          <div className="summary-head">
            <h2>The short version</h2>
            <ReadAloud text={[title, it.why, ...(readable.length ? readable : [body])].filter(Boolean).join(". ")} />
          </div>
          {whole.length >= 2 ? (
            <ul>
              {whole.map((b, i) => (
                <li key={i}>
                  {b} <SaveNote story={it.id} title={title} text={b} />
                </li>
              ))}
            </ul>
          ) : readable.length ? (
            <p>
              {readable[0]} <SaveNote story={it.id} title={title} text={readable[0]} />
            </p>
          ) : body && <p>{body}</p>}
          {it.tags && it.tags.length > 0 && (
            <div className="tags">
              {it.tags.map((t) => (
                <span key={t}>#{t}</span>
              ))}
            </div>
          )}
        </section>

        {it.entities && it.entities.length > 0 && (
          <p className="entity-chips" aria-label="People, places and organisations in this story">
            {it.entities.map((name) => (
              <Link key={name} className="chip" href={entityHref(name)}>
                {name}
              </Link>
            ))}
          </p>
        )}

        {concepts.length > 0 && (
          <section aria-label="Concepts in this story" className="concepts">
            <h2>Concepts in this story</h2>
            <dl className="glossary">
              {concepts.map((c) => (
                <div key={c.term}>
                  <dt>
                    <Link href={`/concepts#${conceptId(c.term)}`}>{c.term}</Link>
                  </dt>
                  <dd>{c.body}</dd>
                </div>
              ))}
            </dl>
          </section>
        )}

        {it.also && it.also.length > 0 && (
          <section aria-label={thread ? "The story so far" : "Also covered by"} className="also">
            <div className="also-head">
              <h2>{thread ? "The story so far" : "Also covered by"}</h2>
              <p>{coverageByKind(it.also.map((o) => o.source_type)) || `${it.also.length} other ${it.also.length === 1 ? "outlet" : "outlets"}`}</p>
            </div>
            <ul>
              {(thread ?? it.also).map((o) => (
                <li key={o.id} className={o.id === it.id ? "here" : undefined}>
                  {thread && <time dateTime={o.time}>{shortDay(o.time)}</time>}
                  {o.id === it.id ? <b aria-current="true">{cleanTitle(o.title)}</b> : <Link href={`/story/${encodeURIComponent(o.id)}`}>{cleanTitle(o.title)}</Link>}
                  <span>{o.id === it.id ? "you are here" : hostName(o.source)}</span>
                </li>
              ))}
            </ul>
          </section>
        )}

        {it.exam && <ExamLens lens={it.exam} />}

        <Share id={it.id} title={title} />

        {external && (
          <div className="read-on">
            <a className="btn primary big" href={external} target="_blank" rel="noopener noreferrer">
              Read the full article at {publisher} <span aria-hidden="true">↗</span>
            </a>
            <p>Postpaper writes a short summary of each story and links to the publisher. We are not the publisher.</p>
          </div>
        )}
      </article>

      {related.length > 0 && (
        <section className="story-more">
          <div className="sec-head">
            <h2>More in {catName(category)}</h2>
            <Link className="more" href={`/?category=${encodeURIComponent(category)}`}>
              See all {catName(category)} <span aria-hidden="true">→</span>
            </Link>
          </div>
          <div className="cards">
            {related.map((r) => (
              <Card key={r.id} it={r} now={now} />
            ))}
          </div>
        </section>
      )}
    </>
  );
}

export default function Page({ params }: PageProps<"/story/[id]">) {
  return (
    <main className="content" id="view">
      <Suspense fallback={<div className="sk sk-title" style={{ marginTop: 64 }} />}>
        <Story params={params} />
      </Suspense>
    </main>
  );
}
