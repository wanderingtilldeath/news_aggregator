import india from "@/lib/data/india.json";
import { inIndia, inWorld, project, projectWorld, unproject, unprojectWorld, type MapLayer } from "@/lib/maps";

// An outline a map sits on: its size in SVG units, its shapes (states or countries) and how longitude and latitude become
// a point on it. India's is lib/data/india.json; a world map brings its own (see lib/maps.ts for the projections).
export type Base = {
  id: "india" | "world";
  name: string;
  w: number;
  h: number;
  shapes: { id: string; name: string; d: string }[];
  project: (lon: number, lat: number) => { x: number; y: number };
  unproject: (x: number, y: number) => { lon: number; lat: number };
  contains: (lon: number, lat: number) => boolean;
};

export type WorldOutline = { w: number; h: number; d: string };

// The world's: one silhouette of land with no country borders (see scripts/world-data.mjs for why), so no border is drawn.
export const worldBase = (o: WorldOutline): Base => ({ id: "world", name: "the world", w: o.w, h: o.h, shapes: [{ id: "land", name: "Land", d: o.d }], project: projectWorld, unproject: unprojectWorld, contains: inWorld });

export const baseFor = (map: MapLayer, world?: WorldOutline): Base => (map.base === "world" && world ? worldBase(world) : INDIA_BASE);

export const INDIA_BASE: Base = { id: "india", name: "India", w: india.w, h: india.h, shapes: india.states, project, unproject, contains: inIndia };
