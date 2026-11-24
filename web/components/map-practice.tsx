"use client";

import { useRef, useState, useSyncExternalStore, type PointerEvent } from "react";
import india from "@/lib/data/india.json";
import layerData from "@/lib/data/map-layers.json";
import { readLocal, subscribeLocal, writeLocal } from "@/lib/local";
import { MAPS_KEY, MAX_LABEL, MAX_MARKERS, MAX_NOTE, addMarker, inIndia, parseMarkers, project, removeMarker, unproject, updateMarker, type LayerItem, type MapLayer, type Marker } from "@/lib/maps";

const LAYERS = layerData.layers as MapLayer[];
const GROUPS = [...new Set(LAYERS.map((l) => l.group))];
const MAX_ZOOM = 10;
const clamp = (n: number, lo: number, hi: number) => Math.min(hi, Math.max(lo, n));
const newId = () => `m-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 7)}`;

type View = { z: number; cx: number; cy: number }; // zoom, and the map point at the centre of the picture
const FULL: View = { z: 1, cx: india.w / 2, cy: india.h / 2 };

// The map practice page: an outline of India that a student marks places on, each with a note, plus ready-made layers
// (soils, climate, protected areas, rivers, passes, ports) to study or to copy places from. The markers live in this
// browser only (lib/maps.ts). All text is drawn as text, never as HTML.
export default function MapPractice() {
  const raw = useSyncExternalStore(subscribeLocal, () => readLocal(MAPS_KEY), () => "");
  const ready = useSyncExternalStore(() => () => {}, () => true, () => false);
  const [layerId, setLayerId] = useState<string>("");
  const [view, setView] = useState<View>(FULL);
  const [pending, setPending] = useState<{ lon: number; lat: number } | null>(null);
  const [form, setForm] = useState({ label: "", note: "" });
  const [picked, setPicked] = useState<{ layer: MapLayer; item: LayerItem } | null>(null);
  const svg = useRef<SVGSVGElement>(null);
  const drag = useRef<{ x: number; y: number; cx: number; cy: number; moved: boolean } | null>(null);

  const markers = parseMarkers(raw);
  const save = (next: Marker[]) => writeLocal(MAPS_KEY, JSON.stringify(next));
  const layer = LAYERS.find((l) => l.id === layerId);
  const has = (id: string) => markers.some((m) => m.id === id);

  const vw = india.w / view.z;
  const vh = india.h / view.z;
  const box = `${view.cx - vw / 2} ${view.cy - vh / 2} ${vw} ${vh}`;

  // Keep the picture inside the map when zooming or panning.
  const fit = (z: number, cx: number, cy: number): View => {
    const zz = clamp(z, 1, MAX_ZOOM);
    return { z: zz, cx: clamp(cx, india.w / zz / 2, india.w - india.w / zz / 2), cy: clamp(cy, india.h / zz / 2, india.h - india.h / zz / 2) };
  };
  const zoomBy = (f: number) => setView((v) => fit(v.z * f, v.cx, v.cy));

  // The map point under a screen point.
  const at = (clientX: number, clientY: number) => {
    const el = svg.current;
    const inv = el?.getScreenCTM()?.inverse();
    if (!el || !inv) return null;
    const p = new DOMPoint(clientX, clientY).matrixTransform(inv);
    return { x: p.x, y: p.y };
  };

  const down = (e: PointerEvent<SVGSVGElement>) => {
    drag.current = { x: e.clientX, y: e.clientY, cx: view.cx, cy: view.cy, moved: false };
    e.currentTarget.setPointerCapture(e.pointerId);
  };
  const move = (e: PointerEvent<SVGSVGElement>) => {
    const d = drag.current;
    const el = svg.current;
    if (!d || !el) return;
    const dx = e.clientX - d.x;
    const dy = e.clientY - d.y;
    if (!d.moved && Math.hypot(dx, dy) < 5) return;
    d.moved = true;
    const rect = el.getBoundingClientRect();
    // One screen pixel is vw / width map units (the picture is letterboxed, so use the smaller scale).
    const unit = Math.max(vw / rect.width, vh / rect.height);
    setView((v) => fit(v.z, d.cx - dx * unit, d.cy - dy * unit));
  };
  const up = (e: PointerEvent<SVGSVGElement>) => {
    const d = drag.current;
    drag.current = null;
    if (!d || d.moved) return;
    const p = at(e.clientX, e.clientY);
    if (!p) return;
    const { lon, lat } = unproject(p.x, p.y);
    if (!inIndia(lon, lat)) return;
    setPicked(null);
    setPending({ lon, lat });
    setForm({ label: "", note: "" });
  };

  const commit = () => {
    if (!pending || markers.length >= MAX_MARKERS) return;
    save(addMarker(markers, { id: newId(), ...pending, label: form.label || "My place", note: form.note }));
    setPending(null);
  };
  const addItem = (l: MapLayer, i: LayerItem) => save(addMarker(markers, { id: `${l.id}:${i.id}`, lon: i.lon, lat: i.lat, label: i.name, note: i.detail ?? "" }));
  const clear = () => {
    if (window.confirm("Remove all your markers?")) save([]);
  };

  if (!ready) {
    return (
      <header className="np-head">
        <h1>Map practice</h1>
      </header>
    );
  }

  const dot = 5 / Math.sqrt(view.z);
  const colorOf = (id: string) => layer?.types?.find((t) => t.id === layer.regions?.[id])?.color;

  return (
    <>
      <header className="np-head">
        <div>
          <h1>Map practice</h1>
          <p className="np-count">Tap the map to mark a place and write a note on it. Pick a layer to study soils, climate, reserves, rivers, passes and ports.</p>
        </div>
        {markers.length > 0 && (
          <div className="np-tools" role="group" aria-label="Map actions">
            <button type="button" onClick={() => window.print()} title="Print or save as PDF">
              Print
            </button>
            <button type="button" onClick={clear}>
              Clear all
            </button>
          </div>
        )}
      </header>

      <div className="map-layout">
        <div className="map-stage">
          <svg
            ref={svg}
            viewBox={box}
            role="img"
            aria-label={`Map of India with ${markers.length} of your markers${layer ? ` and the ${layer.title} layer` : ""}`}
            onPointerDown={down}
            onPointerMove={move}
            onPointerUp={up}
            onPointerCancel={() => (drag.current = null)}
            onWheel={(e) => zoomBy(e.deltaY < 0 ? 1.25 : 0.8)}
          >
            {india.states.map((s) => (
              <path key={s.id} d={s.d} className="map-state" style={colorOf(s.id) ? { fill: colorOf(s.id) } : undefined}>
                <title>{s.name}</title>
              </path>
            ))}
            {layer?.items?.map((i) => {
              const p = project(i.lon, i.lat);
              return (
                <circle
                  key={i.id}
                  cx={p.x}
                  cy={p.y}
                  r={dot}
                  className={`map-dot${picked?.item.id === i.id ? " on" : ""}`}
                  style={{ fill: layer.color }}
                  onPointerDown={(e) => e.stopPropagation()}
                  onPointerUp={(e) => {
                    e.stopPropagation();
                    setPending(null);
                    setPicked({ layer, item: i });
                  }}
                >
                  <title>{i.name}</title>
                </circle>
              );
            })}
            {markers.map((m) => {
              const p = project(m.lon, m.lat);
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
              const p = project(pending.lon, pending.lat);
              return <circle cx={p.x} cy={p.y} r={dot * 1.3} className="map-pending" pointerEvents="none" />;
            })()}
          </svg>
          <div className="map-zoom" role="group" aria-label="Zoom">
            <button type="button" onClick={() => zoomBy(1.6)} aria-label="Zoom in">
              +
            </button>
            <button type="button" onClick={() => zoomBy(1 / 1.6)} aria-label="Zoom out">
              −
            </button>
            <button type="button" onClick={() => setView(FULL)} aria-label="Show the whole map">
              ⌂
            </button>
          </div>
        </div>

        <div className="map-side">
          {pending && (
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

          <section className="map-card" aria-label="Layers">
            <h2>Layers</h2>
            <div className="map-layers">
              <button type="button" aria-pressed={!layerId} onClick={() => { setLayerId(""); setPicked(null); }}>
                None
              </button>
              {GROUPS.map((g) => (
                <div key={g}>
                  <h3>{g}</h3>
                  {LAYERS.filter((l) => l.group === g).map((l) => (
                    <button key={l.id} type="button" aria-pressed={layerId === l.id} onClick={() => { setLayerId(l.id); setPicked(null); }}>
                      {l.title}
                    </button>
                  ))}
                </div>
              ))}
            </div>

            {layer && (
              <div className="map-layer-info">
                <p className="fine-print">
                  {layer.note ? `${layer.note} ` : ""}Source: {layer.source}. As of {layer.asOf}.
                </p>
                {layer.types && (
                  <ul className="map-legend">
                    {layer.types.map((t) => (
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
                {layer.items && (
                  <>
                    {picked && picked.layer.id === layer.id && (
                      <p className="map-picked">
                        <strong>{picked.item.name}</strong> · {picked.item.state}
                        {picked.item.detail ? ` · ${picked.item.detail}` : ""}
                      </p>
                    )}
                    <ul className="map-items">
                      {layer.items.map((i) => (
                        <li key={i.id}>
                          <div>
                            <span className="map-swatch" style={{ background: layer.color }} aria-hidden="true" />
                            <strong>{i.name}</strong>
                            <span className="map-meta">{[i.state, i.detail].filter(Boolean).join(" · ")}</span>
                          </div>
                          <button type="button" className="btn" disabled={has(`${layer.id}:${i.id}`) || markers.length >= MAX_MARKERS} onClick={() => addItem(layer, i)} aria-label={`Add ${i.name} to my map`}>
                            {has(`${layer.id}:${i.id}`) ? "Added" : "Add"}
                          </button>
                        </li>
                      ))}
                    </ul>
                  </>
                )}
              </div>
            )}
          </section>

          <section className="map-card" aria-label="My markers">
            <h2>
              My markers <span className="map-n">{markers.length}</span>
            </h2>
            {markers.length === 0 ? (
              <p className="fine-print">Nothing marked yet. Tap the map, or press Add beside a place in a layer.</p>
            ) : (
              <ul className="map-mine">
                {markers.map((m) => (
                  <li key={m.id}>
                    <input type="text" value={m.label} maxLength={MAX_LABEL} aria-label="Place" onChange={(e) => save(updateMarker(markers, m.id, { label: e.target.value }))} />
                    <textarea value={m.note} maxLength={MAX_NOTE} rows={2} aria-label={`Note on ${m.label}`} placeholder="Add a note" onChange={(e) => save(updateMarker(markers, m.id, { note: e.target.value }))} />
                    <div className="map-row">
                      <span className="map-coord">
                        {m.lat.toFixed(2)}°N, {m.lon.toFixed(2)}°E
                      </span>
                      <button type="button" className="btn" onClick={() => setView(fit(4, project(m.lon, m.lat).x, project(m.lon, m.lat).y))}>
                        Show
                      </button>
                      <button type="button" className="btn" onClick={() => save(removeMarker(markers, m.id))} aria-label={`Remove ${m.label}`}>
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
            <p className="fine-print">Kept in this browser only. Clearing your browser data removes them.</p>
          </section>
        </div>
      </div>
    </>
  );
}
