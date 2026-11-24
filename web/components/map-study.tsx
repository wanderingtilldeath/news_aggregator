"use client";

import { useState, useSyncExternalStore } from "react";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { MAPS_KEY, MAX_LABEL, MAX_MARKERS, MAX_NOTE, addMarker, parseMarkers, removeMarker, updateMarker, type LayerItem, type MapLayer, type MapLine, type Marker } from "@/lib/maps";
import { baseFor, type WorldOutline } from "./map-bases";
import MapCanvas, { useMapView } from "./map-canvas";

const newId = () => `m-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`;

// Study mode of one ready-made map: the outline with the map's places drawn on it, the list of them, and the reader's own
// markers with notes (kept in this browser only, lib/maps.ts). A tap on the map marks a place; a tap on one of the map's own
// dots shows its name. Text is drawn as text, never as HTML.
export default function MapStudy({ map, world }: { map: MapLayer; world?: WorldOutline }) {
  const base = baseFor(map, world);
  // Markers are the reader's notes on India's map; a world map has none.
  const markersOn = map.base === "india";
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(MAPS_KEY), () => "");
  const state = useMapView(base);
  const [pending, setPending] = useState<{ lon: number; lat: number } | null>(null);
  const [form, setForm] = useState({ label: "", note: "" });
  const [picked, setPicked] = useState<LayerItem | null>(null);
  const [shape, setShape] = useState("");
  const [line, setLine] = useState<MapLine | null>(null);

  const markers = markersOn ? parseMarkers(raw) : [];
  const save = (next: Marker[]) => writeLocal(MAPS_KEY, JSON.stringify(next));
  const has = (id: string) => markers.some((m) => m.id === id);
  const colorOf = (id: string) => map.types?.find((t) => t.id === map.regions?.[id])?.color;
  // A dot takes the colour of its type when the map has types (fuels of power plants), else the map's colour.
  const dotColor = (i: LayerItem) => map.types?.find((t) => t.id === i.type)?.color ?? map.color;
  const lineColor = (l: MapLine) => map.types?.find((t) => t.id === l.type)?.color ?? map.color;
  const fills = map.kind === "region" ? Object.fromEntries(base.shapes.map((s) => [s.id, colorOf(s.id)])) : undefined;

  const commit = () => {
    if (!pending || markers.length >= MAX_MARKERS) return;
    save(addMarker(markers, { id: newId(), ...pending, label: form.label || "My place", note: form.note }));
    setPending(null);
  };
  const addItem = (i: LayerItem) => save(addMarker(markers, { id: `${map.id}:${i.id}`, lon: i.lon, lat: i.lat, label: i.name, note: i.detail ?? "" }));
  const clear = () => {
    if (window.confirm("Remove all your markers?")) save([]);
  };
  const stateName = base.shapes.find((s) => s.id === shape)?.name;

  return (
    <div className="map-layout">
      <MapCanvas
        base={base}
        state={state}
        fills={fills}
        label={`Map of ${base.name}, ${map.title}${markersOn ? `, with ${markers.length} of your markers` : ""}`}
        onTap={(p) => {
          setPicked(null);
          setLine(null);
          setShape(map.kind === "states" ? (p.shape ?? "") : "");
          if (map.kind === "states" || !markersOn) return;
          setPending({ lon: p.lon, lat: p.lat });
          setForm({ label: "", note: "" });
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
              </g>
            ))}
            {map.items?.map((i) => {
              const p = base.project(i.lon, i.lat);
              return (
                <circle
                  key={i.id}
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
              );
            })}
            {markers.map((m) => {
              const p = base.project(m.lon, m.lat);
              return (
                <g key={m.id} className="map-pin" pointerEvents="none">
                  <circle cx={p.x} cy={p.y} r={dot * 1.1} />
                  <text x={p.x + dot * 1.8} y={p.y + dot * 0.6} fontSize={dot * 2.4}>
                    {m.label}
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
        {markersOn && pending && (
          <form
            className="map-card"
            onSubmit={(e) => {
              e.preventDefault();
              commit();
            }}
          >
            <h2>New marker</h2>
            <p className="map-coord">
              {pending.lat.toFixed(2)}°N, {pending.lon.toFixed(2)}°E
            </p>
            <label>
              Place
              <input type="text" value={form.label} maxLength={MAX_LABEL} autoFocus onChange={(e) => setForm({ ...form, label: e.target.value })} placeholder="Name of the place" />
            </label>
            <label>
              Note
              <textarea value={form.note} maxLength={MAX_NOTE} rows={3} onChange={(e) => setForm({ ...form, note: e.target.value })} placeholder="What to remember about it" />
            </label>
            <div className="map-row">
              <button type="submit" className="btn primary" disabled={markers.length >= MAX_MARKERS}>
                Save marker
              </button>
              <button type="button" className="btn" onClick={() => setPending(null)}>
                Cancel
              </button>
            </div>
            {markers.length >= MAX_MARKERS && <p className="fine-print">You have {MAX_MARKERS} markers, the most this page keeps. Remove one first.</p>}
          </form>
        )}

        <section className="map-card" aria-label={map.title}>
          <h2>{map.title}</h2>
          <p className="fine-print">
            {map.note ? `${map.note} ` : ""}
            {map.approx ? "Approximate. " : ""}Source: {map.source}. Licence: {map.licence}. As of {map.asOf}.
          </p>
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
                    <button type="button" className="btn" aria-pressed={line?.id === l.id} onClick={() => setLine(l)}>
                      Show
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
          {map.items && (
            <>
              {picked && (
                <p className="map-picked">
                  <strong>{picked.name}</strong> · {picked.state}
                  {picked.detail ? ` · ${picked.detail}` : ""}
                </p>
              )}
              <ul className="map-items">
                {map.items.map((i) => (
                  <li key={i.id}>
                    <div>
                      <span className="map-swatch" style={{ background: dotColor(i) }} aria-hidden="true" />
                      <strong>{i.name}</strong>
                      <span className="map-meta">{[i.state, i.detail].filter(Boolean).join(" · ")}</span>
                    </div>
                    <button type="button" className="btn" disabled={has(`${map.id}:${i.id}`) || markers.length >= MAX_MARKERS} onClick={() => addItem(i)} aria-label={`Add ${i.name} to my map`}>
                      {has(`${map.id}:${i.id}`) ? "Added" : "Add"}
                    </button>
                  </li>
                ))}
              </ul>
            </>
          )}
        </section>

        {markersOn && (
        <section className="map-card" aria-label="My markers">
          <h2>
            My markers <span className="map-n">{markers.length}</span>
          </h2>
          {markers.length === 0 ? (
            <p className="fine-print">Nothing marked yet. Tap the map, or press Add beside a place in the list.</p>
          ) : (
            <>
              <ul className="map-mine">
                {markers.map((m) => (
                  <li key={m.id}>
                    <input type="text" value={m.label} maxLength={MAX_LABEL} aria-label="Place" onChange={(e) => save(updateMarker(markers, m.id, { label: e.target.value }))} />
                    <textarea value={m.note} maxLength={MAX_NOTE} rows={2} aria-label={`Note on ${m.label}`} placeholder="Add a note" onChange={(e) => save(updateMarker(markers, m.id, { note: e.target.value }))} />
                    <div className="map-row">
                      <span className="map-coord">
                        {m.lat.toFixed(2)}°N, {m.lon.toFixed(2)}°E
                      </span>
                      <button type="button" className="btn" onClick={() => state.focus(m.lon, m.lat, 4)}>
                        Show
                      </button>
                      <button type="button" className="btn" onClick={() => save(removeMarker(markers, m.id))} aria-label={`Remove ${m.label}`}>
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
              <div className="map-row">
                <button type="button" className="btn" onClick={() => window.print()}>
                  Print
                </button>
                <button type="button" className="btn" onClick={clear}>
                  Clear all
                </button>
              </div>
            </>
          )}
          <p className="fine-print">Kept in this browser only. Clearing your browser data removes them. They show on every map.</p>
        </section>
        )}
      </div>
    </div>
  );
}
