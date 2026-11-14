import Link from "next/link";
import { type Item, imageSrc } from "@/lib/api";
import { ago, catName, cleanTitle, hostName } from "@/lib/view";

// All feed text is rendered as React text (escaped); links pass safeURL.

// A colour derived from a host name, so every site looks the same everywhere.
function hueOf(name: string) {
  let hue = 0;
  for (const ch of name) hue = (hue * 31 + ch.charCodeAt(0)) % 360;
  return hue;
}

// The site's name on a gradient. It sits under the picture: when a site has no image, or the
// image fails to load, the empty <img> shows nothing and the tile is what the reader sees.
export function SiteTile({ source }: { source: string }) {
  const name = hostName(source);
  const hue = hueOf(name);
  const bg = `linear-gradient(135deg, hsl(${hue} 42% 34%), hsl(${(hue + 40) % 360} 42% 24%))`;
  return (
    <div className="img tile" style={{ background: bg }}>
      {name || "news"}
    </div>
  );
}

// Every story opens on its own Postpaper page first (see app/story/[id]); the page has the
// button that goes on to the publisher.
export const storyHref = (it: Item) => `/story/${encodeURIComponent(it.id)}`;

function Media({ it, eager, badge }: { it: Item; eager?: boolean; badge?: boolean }) {
  const src = imageSrc(it);
  return (
    <Link className="media" href={storyHref(it)} tabIndex={-1} aria-hidden="true">
      {badge && <span className="badge">{catName(it.category || "other")}</span>}
      <SiteTile source={it.source} />
      {src && (
        // Plain <img>: pictures come from our own /img proxy; the Next optimizer would make Vercel fetch them again.
        // eslint-disable-next-line @next/next/no-img-element
        <img
          className="img"
          src={src}
          alt=""
          decoding="async"
          loading={eager ? "eager" : "lazy"}
          fetchPriority={eager ? "high" : undefined}
          referrerPolicy="no-referrer"
        />
      )}
    </Link>
  );
}

function Headline({ it, as: Tag = "h3" }: { it: Item; as?: "h1" | "h2" | "h3" }) {
  return (
    <Tag className="headline">
      <Link href={storyHref(it)}>{cleanTitle(it.title || it.url)}</Link>
    </Tag>
  );
}

// The site's initial on its colour, then its name.
function SourceBadge({ source }: { source: string }) {
  const name = hostName(source);
  return (
    <span className="src">
      <span className="dot" style={{ background: `hsl(${hueOf(name)} 45% 38%)` }} aria-hidden="true">
        {name.charAt(0)}
      </span>
      {name}
    </span>
  );
}

// Five bars for the importance score (1 to 5) from the enrichment pass.
export function Meter({ value }: { value?: number }) {
  if (!value) return null;
  return (
    <span className="meter" role="img" aria-label={`Importance ${value} of 5`} title={`Importance ${value} of 5`}>
      {[1, 2, 3, 4, 5].map((n) => (
        <i key={n} className={n <= value ? "on" : ""} />
      ))}
    </span>
  );
}

export function Meta({ it, now }: { it: Item; now: number }) {
  return (
    <div className="meta">
      <SourceBadge source={it.source} />
      <time dateTime={it.time}>{ago(it.time, now)}</time>
      <Meter value={it.importance} />
    </div>
  );
}

// The "why it matters" line from the enrichment pass.
function Why({ it }: { it: Item }) {
  return it.why ? (
    <p className="why">
      <b>Why it matters:</b> {it.why}
    </p>
  ) : null;
}

function Tags({ it }: { it: Item }) {
  return it.tags?.length ? (
    <div className="tags">
      {it.tags.map((t) => (
        <span key={t}>#{t}</span>
      ))}
    </div>
  ) : null;
}

// fullText is the longest readable text we have for a story. A gist bullet that was cut off
// (it ends in an ellipsis) loses to the page's own description when that says more, so a
// card never shows a half sentence when a whole one exists.
export function fullText(it: Item) {
  const bullet = it.summary?.[0]?.trim() ?? "";
  const desc = (it.description ?? "").trim();
  const title = (it.title ?? "").trim();
  if (bullet && !/[…]|\.\.\.$/.test(bullet)) return bullet;
  if (desc && desc !== title && desc.length > bullet.length) return desc;
  return bullet || desc || (it.snippet ?? "").trim();
}

// What a card shows is limited on purpose: a picture with its section, a headline, the text
// in full (never cut by the layout) and the meta row. The why-it-matters line is the text
// when there is one.
function Blurb({ it, mode }: { it: Item; mode?: string }) {
  if (mode === "7" && it.long) return <p className="long">{it.long}</p>;
  if (it.why) {
    return (
      <p className="why">
        <b>Why it matters:</b> {it.why}
      </p>
    );
  }
  if (mode === "1") return null;
  const text = fullText(it);
  return text ? <p className="desc">{text}</p> : null;
}

export function Card({ it, now, eager, mode }: { it: Item; now: number; eager?: boolean; mode?: string }) {
  return (
    <article className="card">
      <Media it={it} eager={eager} badge />
      <div className="body">
        <Headline it={it} />
        <Blurb it={it} mode={mode} />
        <Meta it={it} now={now} />
      </div>
    </article>
  );
}

export function Row({ it, now, withGist, mode }: { it: Item; now: number; withGist?: boolean; mode?: string }) {
  return (
    <article className="row">
      <Media it={it} badge />
      <div className="text">
        <Headline it={it} />
        <Why it={it} />
        {withGist && (mode === "7" && it.long ? <p className="long">{it.long}</p> : fullText(it) && <p className="desc">{fullText(it)}</p>)}
        <Tags it={it} />
        <Meta it={it} now={now} />
      </div>
    </article>
  );
}

export function Feature({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="feature">
      <Media it={it} eager badge />
      <div className="text">
        <Headline it={it} as="h2" />
        <Why it={it} />
        {mode === "7" && it.long ? <p className="long">{it.long}</p> : fullText(it) && <p className="desc">{fullText(it)}</p>}
        <Tags it={it} />
        <Meta it={it} now={now} />
      </div>
    </article>
  );
}

// ---------- newspaper front page pieces ----------

// "By <source> · 2h ago", the way a newspaper credits a story.
export function Byline({ it, now }: { it: Item; now: number }) {
  return (
    <p className="byline-line">
      By <b>{hostName(it.source)}</b> · <time dateTime={it.time}>{ago(it.time, now)}</time>
    </p>
  );
}

// A text-only story for a side column: headline, text, byline.
export function TextStory({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="text-story">
      <Headline it={it} />
      <Blurb it={it} mode={mode} />
      <Byline it={it} now={now} />
    </article>
  );
}

// The main story: a boxed lead with a label, a large picture and headline.
export function MainStory({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="main-story">
      <div className="label-wrap">
        <span className="label">Main story</span>
        <Media it={it} eager />
      </div>
      <Headline it={it} as="h2" />
      <Blurb it={it} mode={mode} />
      {fullText(it) && it.why && <p className="desc">{fullText(it)}</p>}
      <div className="main-meta">
        <Byline it={it} now={now} />
        <Meter value={it.importance} />
      </div>
    </article>
  );
}

// A story with a small picture beside it.
export function ThumbRow({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="thumb-row">
      <Media it={it} />
      <div>
        <Headline it={it} />
        <Blurb it={it} mode={mode} />
        <Byline it={it} now={now} />
      </div>
    </article>
  );
}

// An entry in a numbered list: "#1" in large type, then the headline.
export function RankItem({ it, n, now }: { it: Item; n: number; now: number }) {
  return (
    <li className="rank-item">
      <span className="rank-n num" aria-hidden="true">
        #{n}
      </span>
      <div>
        <Headline it={it} />
        <Byline it={it} now={now} />
      </div>
    </li>
  );
}
