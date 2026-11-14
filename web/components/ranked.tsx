import { safeURL, type Item } from "@/lib/api";
import { TRACKS, TRACK_IDS, catName, hostName, score, trackOf } from "@/lib/view";

// Stories grouped by edition and ranked by importance (newest first among equals). Used by
// the archive and weekly pages, which are meant to be read top to bottom or printed.
export function rankByTrack(items: Item[], perTrack: number) {
  return TRACK_IDS.map((t) => ({
    id: t,
    title: TRACKS[t].title,
    items: items
      .filter((it) => trackOf(it.category ?? "") === t)
      .sort((a, b) => score(b.importance) - score(a.importance) || Date.parse(b.time) - Date.parse(a.time))
      .slice(0, perTrack),
  })).filter((g) => g.items.length > 0);
}

export function RankedSections({ groups }: { groups: ReturnType<typeof rankByTrack> }) {
  return (
    <>
      {groups.map((g) => (
        <section key={g.id}>
          <div className="sectionhead">
            <h1 style={{ fontSize: 22 }}>{g.title}</h1>
          </div>
          <ol className="rank">
            {g.items.map((it) => {
              const href = safeURL(it.url);
              const title = it.title || it.url;
              return (
                <li key={it.id}>
                  <div className="body">
                    <h3 className="headline">
                      {href ? (
                        <a href={href} target="_blank" rel="noopener noreferrer">
                          {title}
                        </a>
                      ) : (
                        title
                      )}
                    </h3>
                    {it.why && (
                      <p className="why">
                        <b>Why it matters:</b> {it.why}
                      </p>
                    )}
                    {!it.why && it.summary?.[0] && <p className="desc">{it.summary[0]}</p>}
                    <div className="meta">
                      <span className="src">{hostName(it.source)}</span>
                      <span>{catName(it.category || "other")}</span>
                      {it.tags?.map((t) => <span key={t}>#{t}</span>)}
                    </div>
                  </div>
                </li>
              );
            })}
          </ol>
        </section>
      ))}
    </>
  );
}
