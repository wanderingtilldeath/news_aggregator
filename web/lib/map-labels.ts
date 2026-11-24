import type { MapLayer } from "./maps";

// The "Labelled" poster: a dot for each place, a line from the dot out to its name in the margin (as on a printed revision
// map). This file only works out where everything goes; components/map-poster.tsx draws it. Pure, so Node tests can import it.

export type Spot = { id: string; name: string; x: number; y: number };

// All places are named when there are this many or fewer; more than that, the first FIRST are named and the rest are
// numbered dots with a key under the picture. The states map is always named in full (its centres are well spread).
export const MAX_ALL = 35;
export const FIRST = 30;

export function chooseLabelled<T>(items: T[], kind: MapLayer["kind"]): { labelled: T[]; keyed: T[] } {
  if (kind === "states" || items.length <= MAX_ALL) return { labelled: items, keyed: [] };
  return { labelled: items.slice(0, FIRST), keyed: items.slice(FIRST) };
}

// A name over two lines at most, broken at the space that leaves the two lines most even; a name too long for that is cut with "…".
export function wrapLabel(name: string, maxChars: number): string[] {
  const text = name.replace(/\s+/g, " ").trim();
  if (text.length <= maxChars) return [text];
  let best = -1;
  for (let i = 1; i < text.length - 1; i++) {
    if (text[i] !== " ") continue;
    if (best < 0 || Math.abs(i - (text.length - i - 1)) < Math.abs(best - (text.length - best - 1))) best = i;
  }
  if (best < 0) return [text.slice(0, maxChars - 1) + "…"];
  const [a, b] = [text.slice(0, best), text.slice(best + 1)];
  const cut = (s: string) => (s.length > maxChars ? s.slice(0, maxChars - 1) + "…" : s);
  return [cut(a), cut(b)];
}

// The poster's own numbers (components/map-poster.tsx and the tests use the same): names start 60 units past the outline, hold
// 30 characters a line at font size 18 (the margin is 360 wide), 21 units a line and 10 between names.
export const LAYOUT = { top: 20, pad: 60, maxChars: 30, lineH: 21, gap: 10, font: 18, margin: 360 };

export type Frame = {
  w: number; // the outline's width: dots lie between 0 and w
  // The rows the names may use, top and bottom (on the picture's own y axis).
  top: number;
  bottom: number;
  // How far past the outline the names start, the characters a line holds, and the height of one line and the gap between names.
  pad: number;
  maxChars: number;
  lineH: number;
  gap: number;
};

export type Callout = {
  spot: Spot;
  lines: string[];
  side: "left" | "right";
  // The middle of the name's block, and the height it needs.
  ly: number;
  h: number;
  // The point where the name starts (its text is anchored here) and the line from the dot to it.
  labelX: number;
  path: [number, number][];
};

// Which side each place's name goes: the left of the outline's middle goes left, the right goes right; when one side
// would hold more than 60%, the places nearest the middle move across.
export function splitSides(spots: Spot[], w: number): { left: Spot[]; right: Spot[] } {
  const left = spots.filter((s) => s.x < w / 2);
  const right = spots.filter((s) => s.x >= w / 2);
  const cap = Math.max(1, Math.ceil(spots.length * 0.6));
  while (left.length > cap) right.push(left.splice(left.reduce((b, s, i) => (s.x > left[b].x ? i : b), 0), 1)[0]);
  while (right.length > cap) left.push(right.splice(right.reduce((b, s, i) => (s.x < right[b].x ? i : b), 0), 1)[0]);
  return { left, right };
}

// Each name sits as near as it can to its dot's height without touching its neighbour: names are taken top to bottom and pushed
// down where they would overlap, and where that runs past the bottom they are pushed back up. The order never changes, so the
// lines out to the names do not cross.
function spread(items: { want: number; h: number }[], top: number, bottom: number, gap: number): number[] {
  const y = items.map((i) => i.want);
  for (let i = 0; i < y.length; i++) {
    y[i] = Math.max(y[i], top + items[i].h / 2);
    if (i) y[i] = Math.max(y[i], y[i - 1] + (items[i - 1].h + items[i].h) / 2 + gap);
  }
  for (let i = y.length - 1; i >= 0; i--) {
    y[i] = Math.min(y[i], bottom - items[i].h / 2);
    if (i < y.length - 1) y[i] = Math.min(y[i], y[i + 1] - (items[i].h + items[i + 1].h) / 2 - gap);
  }
  return y;
}

export function layoutCallouts(spots: Spot[], frame: Frame): Callout[] {
  const { left, right } = splitSides(spots, frame.w);
  const out: Callout[] = [];

  for (const [side, group] of [["left", left], ["right", right]] as const) {
    const sorted = [...group].sort((a, b) => a.y - b.y || a.x - b.x);
    const blocks = sorted.map((s) => {
      const lines = wrapLabel(s.name, frame.maxChars);
      return { s, lines, h: lines.length * frame.lineH };
    });
    const ys = spread(blocks.map((b) => ({ want: b.s.y, h: b.h })), frame.top, frame.bottom, frame.gap);
    blocks.forEach((b, i) => {
      const edge = side === "left" ? -20 : frame.w + 20;
      const labelX = side === "left" ? -frame.pad : frame.w + frame.pad;
      out.push({ spot: b.s, lines: b.lines, side, ly: ys[i], h: b.h, labelX, path: [[b.s.x, b.s.y], [edge, b.s.y], [labelX + (side === "left" ? 6 : -6), ys[i]]] });
    });
  }
  return out;
}

// The states that hold at least one place of the map, by the shape id of the outline (a place carries its state's name).
export function shadedStates(items: { state: string }[] | undefined, shapes: { id: string; name: string }[]): string[] {
  const named = new Set((items ?? []).map((i) => i.state));
  return shapes.filter((s) => named.has(s.name)).map((s) => s.id);
}

// Whether two straight pieces of line cross each other (touching end to end does not count).
export function crosses(a: [[number, number], [number, number]], b: [[number, number], [number, number]]): boolean {
  const o = (p: [number, number], q: [number, number], r: [number, number]) => Math.sign((q[0] - p[0]) * (r[1] - p[1]) - (q[1] - p[1]) * (r[0] - p[0]));
  const [p1, p2] = a;
  const [p3, p4] = b;
  const d = [o(p1, p2, p3), o(p1, p2, p4), o(p3, p4, p1), o(p3, p4, p2)];
  return d[0] * d[1] < 0 && d[2] * d[3] < 0;
}
