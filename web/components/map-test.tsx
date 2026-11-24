"use client";

import { useState, useSyncExternalStore } from "react";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { DEFAULT_TOLERANCE_KM, LINE_TOLERANCE, SCORES_KEY, TEST_LENGTH, distToPath, judgePlace, parseScores, pickQuestions, questionsFor, recordScore, summarise, type Question, type Result } from "@/lib/map-test";
import type { MapLayer } from "@/lib/maps";
import { baseFor, type WorldOutline } from "./map-bases";
import MapCanvas, { useMapView } from "./map-canvas";

// The time, read outside the component (which must not call impure functions while it renders).
const clock = () => Date.now();
// A map unit in kilometres: 36 units are one degree of latitude (see PROJ in lib/maps.ts), about 111 km.
const UNIT_KM = 111 / 36;

// Test mode of one ready-made map: ten places or states are named one at a time, the labels are hidden, and the reader taps
// where each is. The answer is shown with how far off the tap was; at the end the misses are listed and can be asked again.
// The best score of each map is kept in this browser only.
export default function MapTest({ map, world }: { map: MapLayer; world?: WorldOutline }) {
  const base = baseFor(map, world);
  const state = useMapView(base);
  const scoresRaw = useSyncExternalStore(subscribeLocal, () => readLocal(SCORES_KEY), () => "");
  const all = questionsFor(map, base.shapes);
  const tolerance = map.tolerance_km ?? DEFAULT_TOLERANCE_KM;
  const best = parseScores(scoresRaw)[map.id];

  const [asked, setAsked] = useState<Question[]>([]);
  const [results, setResults] = useState<Result[]>([]);
  // What the reader's last answer was, shown until Next: where they tapped (or the state or type chosen) and the verdict.
  const [shown, setShown] = useState<{ tap?: { lon: number; lat: number }; shape?: string; km?: number; correct: boolean } | null>(null);
  const [hint, setHint] = useState(false);
  const [at, setAt] = useState(0); // the question being asked

  const n = asked.length;
  const q = at < n ? asked[at] : null;
  const done = n > 0 && at >= n;
  const total = Math.min(TEST_LENGTH, all.length);

  const start = (from: Question[]) => {
    setAsked(pickQuestions(from, TEST_LENGTH, Math.random));
    setResults([]);
    setAt(0);
    setShown(null);
    setHint(false);
    state.reset();
  };

  const finish = (next: Result[]) => {
    if (next.length !== n) return;
    const sum = summarise(next);
    writeLocal(SCORES_KEY, JSON.stringify(recordScore(parseScores(readLocal(SCORES_KEY)), map.id, sum.score, sum.total, clock())));
  };
  const answer = (r: Omit<Result, "id" | "ask">, extra: NonNullable<typeof shown>) => {
    if (!q || shown) return;
    const next = [...results, { id: q.id, ask: q.ask, ...r }];
    setResults(next);
    setShown(extra);
    finish(next);
    // Show the answer and the tap together: centre between them and zoom in as far as keeps both on the picture.
    if (q.kind === "place" && extra.tap) {
      const a = base.project(q.lon, q.lat);
      const t = base.project(extra.tap.lon, extra.tap.lat);
      const spanX = Math.abs(a.x - t.x) + 80;
      const spanY = Math.abs(a.y - t.y) + 80;
      state.set(state.fit(Math.min(4, base.w / spanX, base.h / spanY), (a.x + t.x) / 2, (a.y + t.y) / 2));
    }
  };
  const next = () => {
    setAt(at + 1);
    setShown(null);
    setHint(false);
    state.reset();
  };

  if (all.length === 0) return <p className="empty">This map has nothing to ask yet. Use Study to browse it.</p>;

  // The intro: what the test is, and the best score so far.
  if (!n) {
    return (
      <div className="map-layout">
        <MapCanvas base={base} state={state} hideNames label={`Map of ${base.name} for the ${map.title} test`} />
        <div className="map-side">
      <section className="map-card mp-test">
        <h2>Test yourself on {map.title.toLowerCase()}</h2>
        <p>
          {total} {map.kind === "states" || map.kind === "region" ? "states" : "places"}, one at a time. The names on the map are hidden: {map.kind === "region" ? "choose the right answer for each state." : `tap where each one is. A tap within ${tolerance} km counts.`}
        </p>
        {best && <p className="map-picked">Your best: {best.best} of {best.total} · played {best.plays} {best.plays === 1 ? "time" : "times"}</p>}
        <div className="map-row">
          <button type="button" className="btn primary" onClick={() => start(all)}>
            Start the test
          </button>
        </div>
      </section>
        </div>
      </div>
    );
  }

  const sum = summarise(results);
  const missed = all.filter((x) => sum.misses.some((m) => m.id === x.id));

  // Region maps are answered by choosing from the legend; the map only shows which state is meant.
  const chooser = q?.kind === "type" && (
    <div className="mp-choices" role="group" aria-label="Choose the answer">
      {map.types?.map((t) => (
        <button
          key={t.id}
          type="button"
          className="btn"
          disabled={!!shown}
          onClick={() => answer({ correct: t.id === q.answer }, { correct: t.id === q.answer, shape: q.stateId })}
        >
          <span className="map-swatch" style={{ background: t.color }} aria-hidden="true" />
          {t.name}
        </button>
      ))}
    </div>
  );

  const marked: Record<string, "right" | "wrong" | "ask"> = {};
  if (q && shown && q.kind === "state") {
    marked[q.stateId] = "right";
    if (shown.shape && shown.shape !== q.stateId) marked[shown.shape] = "wrong";
  }
  if (q && q.kind === "type") marked[q.stateId] = shown ? (shown.correct ? "right" : "wrong") : "ask";

  return (
    <div className="map-layout">
      <MapCanvas
        base={base}
        state={state}
        hideNames
        marked={marked}
        label={`Test map of ${base.name}, ${map.title}`}
        onTap={(p) => {
          if (!q || q.kind === "type" || shown) return;
          if (q.kind === "line") {
            const at = base.project(p.lon, p.lat);
            const units = distToPath(q.d, at.x, at.y);
            const km = Math.round(units * UNIT_KM);
            answer({ correct: units <= LINE_TOLERANCE, km }, { tap: { lon: p.lon, lat: p.lat }, km, correct: units <= LINE_TOLERANCE });
          } else if (q.kind === "place") {
            const j = judgePlace(q, p, tolerance);
            answer({ correct: j.correct, km: j.km }, { tap: { lon: p.lon, lat: p.lat }, km: j.km, correct: j.correct });
          } else if (p.shape) {
            answer({ correct: p.shape === q.stateId }, { shape: p.shape, correct: p.shape === q.stateId });
          }
        }}
      >
        {({ dot }) => (
          <>
            {map.lines?.map((l) => (
              <path key={l.id} d={l.d} className="map-river" style={{ stroke: map.types?.find((t) => t.id === l.type)?.color ?? map.color, opacity: 0.55 }} pointerEvents="none" />
            ))}
            {q?.kind === "line" && shown && (
              <>
                <path d={q.d} className="mp-answer-line" pointerEvents="none" />
                {shown.tap && (() => {
                  const t = base.project(shown.tap.lon, shown.tap.lat);
                  return <circle cx={t.x} cy={t.y} r={dot} className="mp-tap" pointerEvents="none" />;
                })()}
              </>
            )}
            {q?.kind === "place" && shown && (
              <>
                {shown.tap && (() => {
                  const a = base.project(q.lon, q.lat);
                  const t = base.project(shown.tap.lon, shown.tap.lat);
                  return <line x1={a.x} y1={a.y} x2={t.x} y2={t.y} className="mp-miss-line" pointerEvents="none" />;
                })()}
                {shown.tap && (() => {
                  const t = base.project(shown.tap.lon, shown.tap.lat);
                  return <circle cx={t.x} cy={t.y} r={dot} className="mp-tap" pointerEvents="none" />;
                })()}
                {(() => {
                  const a = base.project(q.lon, q.lat);
                  return (
                    <g pointerEvents="none">
                      <circle cx={a.x} cy={a.y} r={dot * 1.4} className="mp-answer" />
                      <text x={a.x + dot * 2} y={a.y + dot * 0.6} fontSize={dot * 2.6} className="mp-answer-text">
                        {q.label}
                      </text>
                    </g>
                  );
                })()}
              </>
            )}
          </>
        )}
      </MapCanvas>

      <div className="map-side">
        {!done && q && (
          <section className="map-card mp-test" aria-live="polite">
            <p className="mp-step">
              Question {at + 1} of {n} · {sum.score} right so far
            </p>
            <h2>{q.ask}</h2>
            {chooser}
            {!shown && q.hint && (
              <button type="button" className="btn" onClick={() => setHint(true)} disabled={hint}>
                {hint ? q.hint : "Show a hint"}
              </button>
            )}
            {!shown && q.kind !== "type" && <p className="fine-print">Tap the map.</p>}
            {shown && (
              <>
                <p className={shown.correct ? "note ok" : "note err"} role="status">
                  {q.kind === "place" && (shown.correct ? `Right. You were ${shown.km} km away.` : `Not quite: ${shown.km} km away. It is ${q.hint ? q.hint.replace(/^In /, "in ") : "marked on the map"}.`)}
                  {q.kind === "line" && (shown.correct ? "Right." : `Not quite: your tap was about ${shown.km} km from it. The ${q.ask.replace("Find the ", "")} is drawn in green.`)}
                  {q.kind === "state" && (shown.correct ? "Right." : `Not quite. ${base.shapes.find((s) => s.id === shown.shape)?.name ?? "That"} is the one you tapped; ${base.shapes.find((s) => s.id === q.stateId)?.name} is shaded green.`)}
                  {q.kind === "type" && (shown.correct ? "Right." : `Not quite. It is ${map.types?.find((t) => t.id === q.answer)?.name}.`)}
                </p>
                <div className="map-row">
                  <button type="button" className="btn primary" onClick={next} autoFocus>
                    {at + 1 >= n ? "See your score" : "Next question"}
                  </button>
                </div>
              </>
            )}
          </section>
        )}

        {done && (
          <section className="map-card mp-test" aria-live="polite">
            <p className="mp-step">Your score</p>
            <h2>
              {sum.score} of {sum.total}
            </h2>
            <p>{sum.score === sum.total ? "A clean sweep." : sum.score >= Math.ceil(sum.total * 0.6) ? "A solid result. The ones you missed are below." : "Worth another look in Study. The ones you missed are below."}</p>
            {sum.misses.length > 0 && (
              <ul className="map-items">
                {sum.misses.map((m) => (
                  <li key={m.id}>
                    <div>
                      <strong>{m.ask.replace(/^Find /, "")}</strong>
                      {m.km !== undefined && <span className="map-meta">{m.km} km off</span>}
                    </div>
                  </li>
                ))}
              </ul>
            )}
            <div className="map-row">
              <button type="button" className="btn primary" onClick={() => start(all)}>
                Try again
              </button>
              {missed.length > 0 && (
                <button type="button" className="btn" onClick={() => start(missed)}>
                  Ask the ones I missed
                </button>
              )}
            </div>
          </section>
        )}
      </div>
    </div>
  );
}
