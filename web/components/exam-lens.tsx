import type { ExamLens as Lens } from "@/lib/api";
import { PAPERS, RELEVANCE_WORDS } from "@/lib/view";

// The exam lens of a UPSC story: how relevant it is to each paper, the dimensions it touches,
// the arguments for and against, the facts to remember for Prelims and a Mains answer skeleton.
// A model wrote it from the article text alone, so the panel says so. Every list is optional:
// a story the model could not read in depth shows only what it has.
export default function ExamLens({ lens }: { lens: Lens }) {
  const scores = [lens.prelims, lens.mains, lens.essay, lens.ethics];
  const has = (l?: string[]) => !!l && l.length > 0;

  return (
    <section className="lens" aria-labelledby="lens-h">
      <div className="lens-head">
        <h2 id="lens-h">Exam lens</h2>
        <p>Written by AI from the article. Check it against the source before you rely on it.</p>
      </div>

      <dl className="relevance">
        {PAPERS.map((paper, i) => (
          <div key={paper} className="rel" data-score={scores[i]}>
            <dt>{paper}</dt>
            <dd>
              <span className="rel-bar" role="img" aria-label={`${paper}: ${RELEVANCE_WORDS[scores[i]]}`}>
                {[1, 2, 3].map((n) => (
                  <i key={n} className={n <= scores[i] ? "on" : ""} />
                ))}
              </span>
              <span className="rel-word">{RELEVANCE_WORDS[scores[i]]}</span>
            </dd>
          </div>
        ))}
      </dl>

      {has(lens.dimensions) && (
        <div className="lens-block">
          <h3>Dimensions</h3>
          <div className="dims">
            {lens.dimensions!.map((d) => (
              <span key={d}>{d}</span>
            ))}
          </div>
        </div>
      )}

      {(has(lens.for) || has(lens.against)) && (
        <div className="lens-block">
          <h3>Mains angle: both sides</h3>
          <div className="sides">
            {has(lens.for) && (
              <div className="side for">
                <h4>For</h4>
                <ul>
                  {lens.for!.map((t, i) => (
                    <li key={i}>{t}</li>
                  ))}
                </ul>
              </div>
            )}
            {has(lens.against) && (
              <div className="side against">
                <h4>Against</h4>
                <ul>
                  {lens.against!.map((t, i) => (
                    <li key={i}>{t}</li>
                  ))}
                </ul>
              </div>
            )}
          </div>
        </div>
      )}

      {has(lens.skeleton) && (
        <div className="lens-block">
          <h3>Answer skeleton</h3>
          <ol className="skeleton">
            {lens.skeleton!.map((t, i) => (
              <li key={i}>{t}</li>
            ))}
          </ol>
        </div>
      )}

      {has(lens.facts) && (
        <div className="lens-block facts-box">
          <h3>Prelims fact box</h3>
          <ul>
            {lens.facts!.map((t, i) => (
              <li key={i}>{t}</li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
