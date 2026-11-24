"use client";

import { useState } from "react";
import { createPortal } from "react-dom";
import { MAX_MARKS, MAX_NAME, MAX_NOTE, addNote, clearMap, exportText, isStarred, marksFor, removeMark, restoreMark, starId, toggleStar, updateMark, type Mark } from "@/lib/map-marks";
import { chooseLabelled } from "@/lib/map-labels";
import { labellable, } from "@/lib/map-print";
import { coverageLine, type LayerItem, type MapLayer, type MapLine } from "@/lib/maps";
import { baseFor, type WorldOutline } from "./map-bases";
import MapCanvas, { useMapView } from "./map-canvas";
import MapSheet from "./map-sheet";
import { useMarks } from "./use-marks";
import { usePrintSheet } from "./use-print-sheet";

const newId = () => `n-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`;
const clock = () => Date.now();
// The text file of the reader's notes, saved through a link made in the browser.
function saveText(text: string, filename: string) {
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([text], { type: "text/plain;charset=utf-8" }));
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 10_000);
}
// First point of a line (where a starred river is ringed): its path starts "M x y".
const lineStart = (d: string) => {
  const [x, y] = (d.match(/-?\d+(?:\.\d+)?/g) ?? []).map(Number);
  return { x, y };
};

// Study mode of one ready-made map: the outline with the map's places drawn on it and the list of them. To revise, the reader
// stars the places they forget (and gives each a memory hook), can drop a free note at a spot, and later tests themselves on
// the starred ones. Marks belong to one map (a switch shows those of all maps), are kept in this browser and, when logged in,
// in the account. Text is drawn as text, never as HTML.
export default function MapStudy({ map, world }: { map: MapLayer; world?: WorldOutline }) {
  const base = baseFor(map, world);
  const [all, save] = useMarks();
  const state = useMapView(base);
  const [placing, setPlacing] = useState(false);
  const [pending, setPending] = useState<{ lon: number; lat: number } | null>(null);
  const [form, setForm] = useState({ label: "", note: "" });
  const [picked, setPicked] = useState<LayerItem | null>(null);
  const [shape, setShape] = useState("");
  const [line, setLine] = useState<MapLine | null>(null);
  const [everyMap, setEveryMap] = useState(false);
  const [undo, setUndo] = useState<Mark | null>(null);
  // "Print blank map": the sheet is put at the end of <body> and the stylesheet hides everything else while it prints.
  const [printing, print] = usePrintSheet();
  // A map named in full prints as boxes at the ends of lines; a crowded one prints numbers and a key.
  const sheet = labellable(map) && chooseLabelled(map.items ?? [], map.kind).keyed.length === 0 ? "boxes" : "numbers";

  const marks = marksFor(all, map.id, map.base, everyMap);
  const starIt = (spot: { id: string; name: string; lon: number; lat: number }) => save(toggleStar(all, map.id, map.base, spot, clock()));
  const itemSpot = (i: LayerItem) => ({ id: i.id, name: i.name, lon: i.lon, lat: i.lat });
  const lineSpot = (l: MapLine) => {
    const p = lineStart(l.d);
    return { id: l.id, name: l.name, ...base.unproject(p.x, p.y) };
  };
  const starred = (id: string) => isStarred(all, map.id, id);
  const markOf = (id: string) => all.find((m) => m.id === starId(map.id, id));
  const colorOf = (id: string) => map.types?.find((t) => t.id === map.regions?.[id])?.color;
  // A dot takes the colour of its type when the map has types (fuels of power plants), else the map's colour.
  const dotColor = (i: LayerItem) => map.types?.find((t) => t.id === i.type)?.color ?? map.color;
  const lineColor = (l: MapLine) => map.types?.find((t) => t.id === l.type)?.color ?? map.color;
  const fills = map.kind === "region" ? Object.fromEntries(base.shapes.map((s) => [s.id, colorOf(s.id)])) : undefined;

  const commit = () => {
    if (!pending) return;
    save(addNote(all, { id: newId(), map: map.id, base: map.base, ...pending, name: form.label || "My place", note: form.note }, clock()));
    setPending(null);
  };
  const remove = (m: Mark) => {
    save(removeMark(all, m.id));
    setUndo(m);
    setTimeout(() => setUndo((u) => (u?.id === m.id ? null : u)), 8000);
  };
  const clear = () => {
    const mine = all.filter((m) => m.map === map.id);
    if (mine.length && window.confirm(`Remove your ${mine.length} marks on this map?`)) save(clearMap(all, map.id));
  };
  const stateName = base.shapes.find((s) => s.id === shape)?.name;
  const titleOf = (id: string) => (id === map.id ? map.title : id);
  // Starred places are rings round the map's own dots; notes (and marks of other maps) are pins with their name.
  const rings = marks.filter((m) => m.item && m.map === map.id);
  const pins = marks.filter((m) => !(m.item && m.map === map.id));

  return (
    <div className="map-layout">
      <MapCanvas
        base={base}
        state={state}
        fills={fills}
        placing={placing}
        label={`Map of ${base.name}, ${map.title}, with ${marks.length} of your marks`}
        onTap={(p) => {
          setPicked(null);
          setLine(null);
          setShape(map.kind === "states" ? (p.shape ?? "") : "");
          // A tap only reads the map. A note goes where the reader taps after pressing "Add a note".
          if (!placing) return;
          setPending({ lon: p.lon, lat: p.lat });
          setForm({ label: "", note: "" });
          setPlacing(false);
        }}
      >
        {({ dot }) => (
          <>
            {map.lines?.map((l) => (
              <g key={l.id}>
                <path d={l.d} className={`map-river${line?.id === l.id ? " on" : ""}`} style={{ stroke: lineColor(l) }} pointerEvents="none" />
                <path
                  d={l.d}
                  className="map-river-hit"
                  onPointerDown={(e) => e.stopPropagation()}
                  onPointerUp={(e) => {
                    e.stopPropagation();
                    setPending(null);
                    setPicked(null);
                    setLine(l);
                  }}
                >
                  <title>{l.name}</title>
                </path>
                {starred(l.id) && <circle cx={lineStart(l.d).x} cy={lineStart(l.d).y} r={dot * 1.9} className="map-star" pointerEvents="none" />}
              </g>
            ))}
            {map.items?.map((i) => {
              const p = base.project(i.lon, i.lat);
              return (
                <g key={i.id}>
                  <circle
                    cx={p.x}
                    cy={p.y}
                    r={dot}
                    className={`map-dot${picked?.id === i.id ? " on" : ""}`}
                    style={{ fill: dotColor(i) }}
                    onPointerDown={(e) => e.stopPropagation()}
                    onPointerUp={(e) => {
                      e.stopPropagation();
                      setPending(null);
                      setPicked(i);
                    }}
                  >
                    <title>{i.name}</title>
                  </circle>
                  {starred(i.id) && <circle cx={p.x} cy={p.y} r={dot * 1.9} className="map-star" pointerEvents="none" />}
                </g>
              );
            })}
            {rings
              .filter((m) => !map.items?.some((i) => i.id === m.item) && !map.lines?.some((l) => l.id === m.item))
              .map((m) => {
                const p = base.project(m.lon, m.lat);
                return <circle key={m.id} cx={p.x} cy={p.y} r={dot * 1.9} className="map-star" pointerEvents="none" />;
              })}
            {pins.map((m) => {
              const p = base.project(m.lon, m.lat);
              return (
                <g key={m.id} className="map-pin" pointerEvents="none">
                  <circle cx={p.x} cy={p.y} r={dot * 1.1} />
                  <text x={p.x + dot * 1.8} y={p.y + dot * 0.6} fontSize={dot * 2.4}>
                    {m.name}
                  </text>
                </g>
              );
            })}
            {pending && (() => {
              const p = base.project(pending.lon, pending.lat);
              return <circle cx={p.x} cy={p.y} r={dot * 1.3} className="map-pending" pointerEvents="none" />;
            })()}
          </>
        )}
      </MapCanvas>

      <div className="map-side">
        <p className="mp-how">
          <strong>Revise:</strong> star the places you forget and add a memory hook, then press <b>Test yourself</b> and choose “my starred places”. Missed answers are starred for you. Print a blank map to write on.
        </p>

        {pending && (
          <form
            className="map-card"
            onSubmit={(e) => {
              e.preventDefault();
              commit();
            }}
          >
            <h2>New note</h2>
            <p className="map-coord">
              {pending.lat.toFixed(2)}°N, {pending.lon.toFixed(2)}°E
            </p>
            <label>
              Place
              <input type="text" value={form.label} maxLength={MAX_NAME} autoFocus onChange={(e) => setForm({ ...form, label: e.target.value })} placeholder="Name of the place" />
            </label>
            <label>
              Note
              <textarea value={form.note} maxLength={MAX_NOTE} rows={3} onChange={(e) => setForm({ ...form, note: e.target.value })} placeholder="What to remember about it" />
            </label>
            <div className="map-row">
              <button type="submit" className="btn primary" disabled={all.length >= MAX_MARKS}>
                Save note
              </button>
              <button type="button" className="btn" onClick={() => setPending(null)}>
                Cancel
              </button>
            </div>
            {all.length >= MAX_MARKS && <p className="fine-print">You have {MAX_MARKS} marks, the most that is kept. Remove one first.</p>}
          </form>
        )}

        <section className="map-card" aria-label={map.title}>
          <h2>{map.title}</h2>
          <p className="fine-print">
            {map.coverage ? `${coverageLine(map.coverage, map.items?.length ?? 0)}. ` : ""}
            {map.note ? `${map.note} ` : ""}
            {map.approx ? "Approximate. " : ""}Source: {map.source}. Licence: {map.licence}. As of {map.asOf}.
          </p>
          <div className="map-row">
            <button type="button" className="btn" onClick={print}>
              Print blank map
            </button>
          </div>
          {map.kind === "states" && (
            <>
              <p className="map-picked">{stateName ? <strong>{stateName}</strong> : "Tap a state to see its name."}</p>
              <ul className="map-items">
                {base.shapes.map((s) => (
                  <li key={s.id}>
                    <div>
                      <strong>{s.name}</strong>
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
          {map.types && (
            <ul className="map-legend">
              {map.types.map((t) => (
                <li key={t.id}>
                  <span className="map-swatch" style={{ background: t.color }} aria-hidden="true" />
                  <div>
                    <strong>{t.name}</strong>
                    <span>{t.also}</span>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {map.lines && (
            <>
              <p className="map-picked">{line ? <strong>{line.name}</strong> : "Tap a river, or choose one from the list."}</p>
              <ul className="map-items">
                {map.lines.map((l) => (
                  <li key={l.id}>
                    <div>
                      <span className="map-swatch" style={{ background: lineColor(l) }} aria-hidden="true" />
                      <strong>{l.name}</strong>
                    </div>
                    <div className="map-row">
                      <button type="button" className="btn" aria-pressed={line?.id === l.id} onClick={() => setLine(l)}>
                        Show
                      </button>
                      <button type="button" className="btn mp-star-btn" aria-pressed={starred(l.id)} onClick={() => starIt(lineSpot(l))} aria-label={`${starred(l.id) ? "Unstar" : "Star"} ${l.name} to revise`}>
                        {starred(l.id) ? "★" : "☆"}
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            </>
          )}
          {map.items && (
            <>
              {picked && (
                <div className="map-picked">
                  <strong>{picked.name}</strong> · {picked.state}
                  {picked.detail ? ` · ${picked.detail}` : ""}
                  {starred(picked.id) && (
                    <label className="mp-hook">
                      Your memory hook
                      <textarea value={markOf(picked.id)?.note ?? ""} maxLength={MAX_NOTE} rows={2} placeholder="A rhyme, a link, why it matters" onChange={(e) => save(updateMark(all, starId(map.id, picked.id), { note: e.target.value }))} />
                    </label>
                  )}
                </div>
              )}
              <ul className="map-items">
                {map.items.map((i) => (
                  <li key={i.id}>
                    <div>
                      <span className="map-swatch" style={{ background: dotColor(i) }} aria-hidden="true" />
                      <strong>{i.name}</strong>
                      <span className="map-meta">{[i.state, i.detail].filter(Boolean).join(" · ")}</span>
                    </div>
                    <button type="button" className="btn mp-star-btn" aria-pressed={starred(i.id)} disabled={!starred(i.id) && all.length >= MAX_MARKS} onClick={() => starIt(itemSpot(i))} aria-label={`${starred(i.id) ? "Unstar" : "Star"} ${i.name} to revise`}>
                      {starred(i.id) ? "★ Revising" : "☆ Revise"}
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>

        <section className="map-card" aria-label="My notes">
          <h2>
            My notes <span className="map-n">{marks.length}</span>
          </h2>
          <div className="map-row">
            <button type="button" className="btn" aria-pressed={placing} onClick={() => setPlacing(!placing)}>
              {placing ? "Tap the map…" : "Add a note on the map"}
            </button>
            <label className="mp-all">
              <input type="checkbox" checked={everyMap} onChange={(e) => setEveryMap(e.target.checked)} /> Show marks from all maps
            </label>
          </div>
          {placing && <p className="fine-print">Tap the map where the note goes. Press the button again to cancel.</p>}
          {undo && (
            <p className="note" role="status">
              Removed “{undo.name}”.{" "}
              <button
                type="button"
                className="btn"
                onClick={() => {
                  save(restoreMark(all, undo));
                  setUndo(null);
                }}
              >
                Undo
              </button>
            </p>
          )}
          {marks.length === 0 ? (
            <p className="fine-print">Nothing here yet. Press ☆ beside a place in the list to revise it, or add a note on the map.</p>
          ) : (
            <>
              <ul className="map-mine">
                {marks.map((m) => (
                  <li key={m.id}>
                    <input type="text" value={m.name} maxLength={MAX_NAME} aria-label="Place" onChange={(e) => save(updateMark(all, m.id, { name: e.target.value }))} />
                    <span className="map-coord">
                      {m.from === "test" ? "★ missed in a test" : m.item ? "★ starred" : "note"}
                      {m.map !== map.id && m.map ? ` · ${titleOf(m.map)}` : ""}
                    </span>
                    <textarea value={m.note} maxLength={MAX_NOTE} rows={2} aria-label={`Note on ${m.name}`} placeholder="Add a memory hook or note" onChange={(e) => save(updateMark(all, m.id, { note: e.target.value }))} />
                    <div className="map-row">
                      <button type="button" className="btn" onClick={() => state.focus(m.lon, m.lat, 4)}>
                        Show
                      </button>
                      <button type="button" className="btn" onClick={() => remove(m)} aria-label={`Remove ${m.name}`}>
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
              <div className="map-row">
                <button type="button" className="btn" onClick={() => saveText(exportText(marks, titleOf), `${map.id}-my-notes.txt`)}>
                  Download my notes
                </button>
                <button type="button" className="btn" onClick={() => window.print()}>
                  Print
                </button>
                <button type="button" className="btn" onClick={clear}>
                  Clear this map&apos;s marks
                </button>
              </div>
            </>
          )}
          <p className="fine-print">Kept in this browser{" "}and, when you are logged in, in your account. Each map has its own marks.</p>
        </section>
      </div>
      {printing && createPortal(<MapSheet map={map} base={base} mode={sheet} marks={marks} />, document.body)}
    </div>
  );
}
