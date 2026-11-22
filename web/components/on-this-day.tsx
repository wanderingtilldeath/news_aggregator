import { getHistory } from "@/lib/api";
import { safeURL } from "@/lib/safe";

// "Today in history": up to three lines from Wikipedia's On This Day feed, written by the crawl job. It renders
// nothing when the file is missing or old (a failed fetch, a stale deploy), so a bad day leaves no hole. Wikipedia's
// text is CC BY-SA, so the section says where it comes from and links each line to its article.
export default async function OnThisDay({ now }: { now: number }) {
  const h = await getHistory().catch(() => null);
  if (!h?.items?.length) return null;

  // A file from another day is worse than none: the strip says "today".
  const today = new Date(now).toLocaleDateString("en-CA", { timeZone: "Asia/Kolkata" });
  if (h.date !== today) return null;

  return (
    <section className="history" aria-label="Today in history">
      <div className="sec-head">
        <h2>Today in history</h2>
      </div>
      <ul>
        {h.items.map((e) => {
          const href = safeURL(e.url);
          return (
            <li key={`${e.year}-${e.text.slice(0, 20)}`}>
              <b>{e.year}</b>
              <span>
                {e.text}
                {href && (
                  <>
                    {" "}
                    <a href={href} target="_blank" rel="noopener noreferrer">
                      Read more
                    </a>
                  </>
                )}
              </span>
            </li>
          );
        })}
      </ul>
      <p className="fine-print">
        From <a href="https://en.wikipedia.org/wiki/Wikipedia:On_this_day" target="_blank" rel="noopener noreferrer">Wikipedia</a>, under CC BY-SA.
      </p>
    </section>
  );
}
