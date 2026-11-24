import { cacheLife } from "next/cache";
import { getDigest } from "./api";
import india from "./data/india.json";
import { gazetteer, matchStories } from "./map-news";
import { MAPS } from "./maps-registry";
import { project } from "./maps";

// What the "In the news" map and the strip under each map need: the places of the India maps that the last week's stories
// name, with their position on the outline and up to five stories each. Computed here, on the server, so the browser gets
// a few kilobytes and not the map files.
export type NewsPlace = {
  key: string;
  name: string;
  on: string[];
  count: number;
  x?: number;
  y?: number;
  shape?: string;
  d?: string;
  stories: { id: string; title: string; source: string }[];
};

async function compute(): Promise<NewsPlace[]> {
  "use cache";
  cacheLife({ stale: 300, revalidate: 1800, expire: 86400 });
  // The same call as the sitemap's, so both share one cached answer.
  const { items } = await getDigest({ days: "7", pageSize: 200 });
  const places = gazetteer(Object.values(MAPS), india.states);
  return matchStories(places, items).map(({ place, count, stories }) => {
    const p = place.lon !== undefined && place.lat !== undefined ? project(place.lon, place.lat) : undefined;
    return { key: place.key, name: place.name, on: place.on, count, x: p?.x, y: p?.y, shape: place.shape, d: place.d, stories: stories.map((s) => ({ id: s.id, title: s.title, source: s.source })) };
  });
}

// null when the API cannot be reached (the page then says so and the strip is left out).
export const getMapNews = () => compute().catch(() => null);
