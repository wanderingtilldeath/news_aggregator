import { getIntro } from "@/lib/api";
import { TRACKS, TRACK_IDS } from "@/lib/view";
import ReadAloud from "./read-aloud";

// "Today in 60 seconds": the day's one-paragraph briefing, one per edition. It is written once a day
// by the crawl job, so a missing one (an early crawl, a day with too few stories) just hides the box.
export default async function Briefing({ track }: { track: string }) {
  const ids = track ? [track] : TRACK_IDS;
  const intros = (await Promise.all(ids.map((id) => getIntro(id).catch(() => null)))).flatMap((intro, i) => (intro ? [{ id: ids[i], text: intro.text }] : []));
  if (!intros.length) return null;

  return (
    <section className="briefing" aria-labelledby="briefing-h">
      <div className="briefing-head">
        <h2 id="briefing-h">Today in 60 seconds</h2>
        <ReadAloud text={intros.map((i) => i.text).join(" ")} label="Listen to the briefing" />
      </div>
      {intros.map((i) => (
        <p key={i.id}>
          {intros.length > 1 && <b>{TRACKS[i.id].title}. </b>}
          {i.text}
        </p>
      ))}
    </section>
  );
}
