import { type Item, imageSrc, safeURL } from "@/lib/api";
import { ago, catName, hostName } from "@/lib/view";

// All feed text is rendered as React text (escaped); links pass safeURL.

// The site's name on a colour derived from its host, so every site looks consistent.
// It sits under the picture: when a site has no image, or the image fails to load,
// the empty <img> shows nothing and the tile is what the reader sees.
function SiteTile({ source }: { source: string }) {
  const name = hostName(source);
  let hue = 0;
  for (const ch of name) hue = (hue * 31 + ch.charCodeAt(0)) % 360;
  const bg = `linear-gradient(135deg, hsl(${hue} 42% 32%), hsl(${(hue + 40) % 360} 42% 24%))`;
  return (
    <div className="img tile" style={{ background: bg }}>
      {name || "news"}
    </div>
  );
}

function Media({ it, eager }: { it: Item; eager?: boolean }) {
  const href = safeURL(it.url);
  const src = imageSrc(it);
  const inner = (
    <>
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
    </>
  );
  return href ? (
    <a className="media" href={href} target="_blank" rel="noopener noreferrer" tabIndex={-1} aria-hidden="true">
      {inner}
    </a>
  ) : (
    <div className="media" aria-hidden="true">
      {inner}
    </div>
  );
}

function Headline({ it, as: Tag = "h3" }: { it: Item; as?: "h2" | "h3" }) {
  const href = safeURL(it.url);
  const text = it.title || it.url;
  return (
    <Tag className="headline">
      {href ? (
        <a href={href} target="_blank" rel="noopener noreferrer">
          {text}
        </a>
      ) : (
        text
      )}
    </Tag>
  );
}

const Kicker = ({ it }: { it: Item }) => <span className="kicker">{catName(it.category || "other")}</span>;

export function Meta({ it, now }: { it: Item; now: number }) {
  return (
    <div className="meta">
      <span className="src">{hostName(it.source)}</span>
      <time dateTime={it.time}>{ago(it.time, now)}</time>
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

// mode "1" is the one-minute read (no summary, the why line carries it), "7" the long
// summary, and "" the standard gist bullets.
function Gist({ it, max = 3, mode = "" }: { it: Item; max?: number; mode?: string }) {
  if (mode === "1" && it.why) return null;
  if (mode === "7" && it.long) return <p className="long">{it.long}</p>;
  if (it.summary?.length) {
    return (
      <ul className="gist">
        {it.summary.slice(0, max).map((b, i) => (
          <li key={i}>{b}</li>
        ))}
      </ul>
    );
  }
  const text = it.description || it.snippet;
  return text ? <p className="desc">{text}</p> : null;
}

// A card is a picture over a headline; a row is a small picture beside one.
export function Card({ it, now, eager, mode }: { it: Item; now: number; eager?: boolean; mode?: string }) {
  return (
    <article className="card">
      <Media it={it} eager={eager} />
      <Kicker it={it} />
      <Headline it={it} />
      <Why it={it} />
      <Gist it={it} mode={mode} />
      <Tags it={it} />
      <Meta it={it} now={now} />
    </article>
  );
}

export function Row({ it, now, withGist, mode }: { it: Item; now: number; withGist?: boolean; mode?: string }) {
  return (
    <article className="row">
      <Media it={it} />
      <div className="text">
        <Kicker it={it} />
        <Headline it={it} />
        <Why it={it} />
        {withGist && <Gist it={it} max={2} mode={mode} />}
        <Tags it={it} />
        <Meta it={it} now={now} />
      </div>
    </article>
  );
}

export function Lead({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="lead">
      <Media it={it} eager />
      <Kicker it={it} />
      <Headline it={it} as="h2" />
      <Why it={it} />
      <Gist it={it} max={4} mode={mode} />
      <Tags it={it} />
      <Meta it={it} now={now} />
    </article>
  );
}

export function Feature({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="feature">
      <Media it={it} eager />
      <div className="text">
        <Kicker it={it} />
        <Headline it={it} as="h2" />
        <Why it={it} />
        <Gist it={it} max={4} mode={mode} />
        <Tags it={it} />
        <Meta it={it} now={now} />
      </div>
    </article>
  );
}
