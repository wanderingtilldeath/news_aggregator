import Link from "next/link";
import { notFound } from "next/navigation";
import { connection } from "next/server";
import { Suspense } from "react";
import { Card, Meter, Signals, SiteTile, fullText } from "@/components/article";
import ExamLens from "@/components/exam-lens";
import ReadAloud from "@/components/read-aloud";
import Share from "@/components/share";
import { getDigest, getStory, imageSrc, safeURL } from "@/lib/api";
import { SOURCE_TYPES, TRACKS, ago, catName, cleanTitle, coverageByKind, hostName, trackOf } from "@/lib/view";

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
  const blurb = (it.why || body).slice(0, 180);

  // More from the same section, from the same cached call the section page makes.
  const more = await getDigest({ days: "7", category, pageSize: 8 }).catch(() => null);
  const related = (more?.items ?? []).filter((x) => x.id !== it.id).slice(0, 3);

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
            <ReadAloud text={[title, it.why, ...bullets].filter(Boolean).join(". ")} />
          </div>
          {bullets.length >= 2 || (bullets.length === 1 && !/…|\.\.\.$/.test(bullets[0])) ? (
            <ul>
              {bullets.map((b, i) => (
                <li key={i}>{b}</li>
              ))}
            </ul>
          ) : (
            body && <p>{body}</p>
          )}
          {it.tags && it.tags.length > 0 && (
            <div className="tags">
              {it.tags.map((t) => (
                <span key={t}>#{t}</span>
              ))}
            </div>
          )}
        </section>

        {it.also && it.also.length > 0 && (
          <section aria-label="Also covered by" className="also">
            <h2>Also covered by</h2>
            <p className="also-kinds">Reported by {coverageByKind([it.source_type, ...it.also.map((o) => o.source_type)])}.</p>
            <ul>
              {it.also.map((o) => (
                <li key={o.id}>
                  <Link href={`/story/${encodeURIComponent(o.id)}`}>{cleanTitle(o.title)}</Link>
                  <span>{hostName(o.source)}</span>
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
