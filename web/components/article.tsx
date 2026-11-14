import { type Item, imageSrc, safeURL } from "@/lib/api";
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
function SiteTile({ source }: { source: string }) {
  const name = hostName(source);
  const hue = hueOf(name);
  const bg = `linear-gradient(135deg, hsl(${hue} 42% 34%), hsl(${(hue + 40) % 360} 42% 24%))`;
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

function Headline({ it, as: Tag = "h3" }: { it: Item; as?: "h1" | "h2" | "h3" }) {
  const href = safeURL(it.url);
  const text = cleanTitle(it.title || it.url);
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
      <div className="body">
        <Kicker it={it} />
        <Headline it={it} />
        <Why it={it} />
        <Gist it={it} max={2} mode={mode} />
        <Tags it={it} />
        <Meta it={it} now={now} />
      </div>
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

// The lead story of the Daily Briefing.
export function Lead({ it, now, mode }: { it: Item; now: number; mode?: string }) {
  return (
    <article className="lead-card">
      <Media it={it} eager />
      <div className="body">
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
