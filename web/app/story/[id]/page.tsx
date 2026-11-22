import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { connection } from "next/server";
import { Suspense } from "react";
import Loading from "@/components/loading";
import { Card, Importance, Signals, SiteTile, fullText } from "@/components/article";
import ReadAloud from "@/components/read-aloud";
import BookmarkButton from "@/components/bookmark-button";
import ReadProgress from "@/components/read-progress";
import ReadingControls from "@/components/reading-controls";
import SaveNote from "@/components/save-note";
import Banner from "@/components/hero";
import Share from "@/components/share";
import { getConcepts, getDigest, getStory, imageSrc, safeURL, type Item } from "@/lib/api";
import { SITE_URL } from "@/lib/site";
import { conceptId, mentioned } from "@/lib/concepts";
import { keyNumbers } from "@/lib/numbers";
import { oneEach, sameStory } from "@/lib/stories";
import { SOURCE_TYPES, TRACKS, ago, catName, cleanTitle, coverageByKind, hostName, readMinutes, trackOf } from "@/lib/view";

const shortDay = (iso?: string) => (iso ? new Date(iso).toLocaleDateString("en-IN", { day: "numeric", month: "short", timeZone: "Asia/Kolkata" }) : "");

// The title, description and share preview of a story. They are made here, not in the page, so they are in the
// head of the first response a crawler reads. A story with nothing of our own on it (no summary, no why line,
// no lead) is kept out of the index: a page that only repeats a headline helps no one.
export async function generateMetadata({ params }: PageProps<"/story/[id]">): Promise<Metadata> {
  const { id } = await params;
  const it = await getStory(id).catch(() => null);
  if (!it) return { title: "Story not found · Postpaper", robots: { index: false } };

  const title = cleanTitle(it.title || it.url);
  const blurb = (it.why || fullText(it)).slice(0, 180);
  const image = imageSrc(it);
  const thin = !it.why && !(it.summary ?? []).length && !it.lead;
  return {
    title: `${title} · Postpaper`,
    description: blurb || undefined,
    alternates: { canonical: `/story/${encodeURIComponent(id)}` },
    robots: thin ? { index: false, follow: true } : undefined,
    openGraph: { type: "article", title, description: blurb || undefined, publishedTime: it.time, siteName: "Postpaper", locale: "en_IN", images: image ? [image] : undefined },
    twitter: { card: image ? "summary_large_image" : "summary", title, description: blurb || undefined },
  };
}

// The story as a NewsArticle for search engines. It says what the page is (a summary with a link on to the
// publisher's article, which isBasedOn names), not that Postpaper wrote the reporting.
function articleLD(it: Item, title: string, blurb: string, image: string | undefined, id: string) {
  return JSON.stringify({
    "@context": "https://schema.org",
    "@type": "NewsArticle",
    headline: title.slice(0, 110),
    description: blurb || undefined,
    datePublished: it.time || undefined,
    image: image ? [image] : undefined,
    inLanguage: "en",
    isBasedOn: safeURL(it.url) || undefined,
    mainEntityOfPage: `${SITE_URL}/story/${encodeURIComponent(id)}`,
    publisher: { "@type": "Organization", name: "Postpaper", url: SITE_URL },
  }).replace(/</g, "\\u003c");
}

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
  // Figures named in the text the reader sees, found with rules (see lib/numbers.ts).
  const figures = keyNumbers([it.long, ...(it.summary ?? []), it.lead].filter(Boolean).join(" "));
  const extra = it.extra;
  const hasExtra = Boolean(extra?.context || extra?.impact || extra?.next?.text);

  // More from the same section, from the same cached call the section page makes.
  const more = await getDigest({ days: "7", category, pageSize: 8 }).catch(() => null);
  const related = oneEach((more?.items ?? []).filter((x) => x.id !== it.id && !sameStory(x, it))).slice(0, 3);
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
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: articleLD(it, title, blurb, image, id) }} />
      <ReadProgress />
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
            {readMinutes(it.words) > 0 && <> · {readMinutes(it.words)} min read</>}
          </span>
          <span className="signals">
            <Signals it={it} />
          </span>
          <Importance value={it.importance} />
        </div>
        <div className="story-tools">
          <ReadingControls />
          <BookmarkButton id={it.id} title={title} source={it.source} time={it.time} />
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

        {hasExtra && (
          <dl className="extras" aria-label="Background and what to watch">
            {extra?.context && (
              <div>
                <dt>Background</dt>
                <dd>{extra.context}</dd>
              </div>
            )}
            {extra?.impact && (
              <div>
                <dt>Who it affects</dt>
                <dd>{extra.impact}</dd>
              </div>
            )}
            {extra?.next?.text && (
              <div>
                <dt>What to watch</dt>
                <dd>
                  {extra.next.date && <b className="when">{shortDay(extra.next.date)}</b>} {extra.next.text}
                </dd>
              </div>
            )}
          </dl>
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

        {figures.length > 0 && (
          <section className="glance" aria-label="At a glance">
            <h2>At a glance</h2>
            <ul>
              {figures.map((f) => (
                <li key={f.figure}>
                  <b>{f.figure}</b>
                  {f.label && <span>{f.label}</span>}
                </li>
              ))}
            </ul>
          </section>
        )}

        {it.entities && it.entities.length > 0 && (
          <p className="entity-chips" aria-label="People, places and organisations in this story">
            {it.entities.map((name) => (
              <span key={name} className="chip">
                {name}
              </span>
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

        <Share path={`/story/${it.id}`} title={title} label="Share this story" />

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

      <Banner />
    </>
  );
}

export default function Page({ params }: PageProps<"/story/[id]">) {
  return (
    <main className="content" id="view">
      <Suspense fallback={<Loading />}>
        <Story params={params} />
      </Suspense>
    </main>
  );
}
