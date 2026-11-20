// Builds the data files of the games in lib/data/ (run once, commit the output; the games read the JSON).
//   mkdir geo && cd geo && npm init -y && npm i world-atlas@2.0.2 topojson-client@3.1.0   (outside the project)
//   node scripts/game-data.mjs <google-10000-english-no-swears.txt> <geo dir>
// Inputs: a list of common English words, the macOS /usr/share/dict/web2 list (to drop non-words and to accept
// more four-letter words in the ladder), and Natural Earth country shapes (world-atlas, public domain).
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

const [commonFile, geoDir] = process.argv.slice(2);
const read = (f) => readFileSync(f, "utf8").split("\n").map((w) => w.trim()).filter(Boolean);
const web2 = new Set(read("/usr/share/dict/web2").filter((w) => /^[a-z]+$/.test(w)));
const out = (name, data) => writeFileSync(new URL(`../lib/data/${name}`, import.meta.url), JSON.stringify(data));

// A small seeded generator so a rebuild gives the same puzzles.
let seed = 20261120;
const rand = () => ((seed = (seed * 1664525 + 1013904223) % 4294967296) / 4294967296);
const shuffle = (a) => { for (let i = a.length - 1; i > 0; i--) { const j = Math.floor(rand() * (i + 1)); [a[i], a[j]] = [a[j], a[i]]; } return a; };

// Common words that are real words: in the dictionary, or an -s/-es/-ed/-ing/-er/-ly form of one.
const inDict = (w) => web2.has(w) || [["s", ""], ["es", ""], ["ed", ""], ["ed", "e"], ["ing", ""], ["ing", "e"], ["er", ""], ["ly", ""], ["ies", "y"]].some(([suf, add]) => w.endsWith(suf) && w.length > suf.length + 2 && web2.has(w.slice(0, -suf.length) + add));
const common = [...new Set(read(commonFile).filter((w) => /^[a-z]+$/.test(w) && w.length >= 3 && inDict(w)))];
out("common-words.json", common);

// Spelling Bee: seven distinct letters (no "s"), a centre letter that gives 15 to 60 words from the common list.
const bee = [];
const seen = new Set();
for (const p of common) {
  const set = [...new Set(p)];
  if (set.length !== 7 || p.includes("s")) continue;
  const key = [...set].sort().join("");
  if (seen.has(key)) continue;
  seen.add(key);
  const words = common.filter((w) => w.length >= 4 && [...w].every((c) => key.includes(c)));
  const centres = [...key].filter((c) => { const n = words.filter((w) => w.includes(c)).length; return n >= 15 && n <= 60; });
  if (centres.length) bee.push({ letters: key, center: centres[Math.floor(rand() * centres.length)] });
}
out("bee-puzzles.json", shuffle(bee));

// Word ladder: puzzles between common four-letter words with a shortest path of 3 to 6 steps through common words.
// Any dictionary word is a legal step, so the valid list is bigger.
const four = common.filter((w) => w.length === 4);
const valid4 = [...new Set([...four, ...[...web2].filter((w) => w.length === 4)])].sort();
out("words4.json", valid4);
const near = (a, b) => { let d = 0; for (let i = 0; i < 4; i++) if (a[i] !== b[i] && ++d > 1) return false; return d === 1; };
const adj = new Map(four.map((w) => [w, four.filter((x) => near(w, x))]));
const ladders = [];
for (const start of shuffle([...four])) {
  const dist = new Map([[start, 0]]);
  const q = [start];
  for (let i = 0; i < q.length; i++) for (const n of adj.get(q[i])) if (!dist.has(n)) { dist.set(n, dist.get(q[i]) + 1); q.push(n); }
  const ends = [...dist].filter(([, d]) => d >= 3 && d <= 6);
  if (ends.length) { const [to, par] = ends[Math.floor(rand() * ends.length)]; ladders.push({ from: start, to, par }); }
  if (ladders.length >= 400) break;
}
out("ladders.json", ladders);

// Countries: the outline as one SVG path in a 100-unit box (width scaled by cos(latitude) so shapes look right),
// the centre, and the area. India, Pakistan and China are in the list to guess but never drawn: their borders are
// disputed and India restricts maps that show them differently from the official one.
const require = createRequire(path.join(geoDir, "x.js"));
const topo = require("topojson-client");
const atlas = JSON.parse(readFileSync(path.join(geoDir, "node_modules/world-atlas/countries-110m.json"), "utf8"));
const RENAME = { "Dem. Rep. Congo": "DR Congo", "Central African Rep.": "Central African Republic", "Dominican Rep.": "Dominican Republic", "Eq. Guinea": "Equatorial Guinea", "S. Sudan": "South Sudan", "Solomon Is.": "Solomon Islands", "Bosnia and Herz.": "Bosnia and Herzegovina", "United States of America": "United States", "eSwatini": "Eswatini", "Macedonia": "North Macedonia", "Czechia": "Czech Republic", "Côte d'Ivoire": "Ivory Coast", "Timor-Leste": "East Timor" };
const SKIP = new Set(["Antarctica", "Fr. S. Antarctic Lands", "N. Cyprus", "Somaliland", "Falkland Is.", "Kosovo", "W. Sahara"]);
const NO_DRAW = new Set(["India", "Pakistan", "China"]);
const countries = [];
for (const f of topo.feature(atlas, atlas.objects.countries).features) {
  const name = RENAME[f.properties.name] ?? f.properties.name;
  if (SKIP.has(f.properties.name)) continue;
  const polys = f.geometry.type === "Polygon" ? [f.geometry.coordinates] : f.geometry.coordinates;
  const pts = polys.flat(2);
  const wraps = pts.some(([x]) => x > 170) && pts.some(([x]) => x < -170); // Russia, Fiji: draw across the date line
  const fix = (r) => r.map(([x, y]) => [wraps && x < 0 ? x + 360 : x, y]);
  const rings = polys.map((p) => p.map(fix));
  const all = rings.flat(2);
  const [minX, maxX] = [Math.min(...all.map((p) => p[0])), Math.max(...all.map((p) => p[0]))];
  const [minY, maxY] = [Math.min(...all.map((p) => p[1])), Math.max(...all.map((p) => p[1]))];
  const k = Math.cos((((minY + maxY) / 2) * Math.PI) / 180);
  const w = (maxX - minX) * k, h = maxY - minY;
  const s = 100 / Math.max(w, h, 0.0001);
  const d = NO_DRAW.has(name) ? "" : rings.map((poly) => poly.map((ring) => "M" + ring.map(([x, y]) => `${((x - minX) * k * s).toFixed(1)} ${((maxY - y) * s).toFixed(1)}`).join("L") + "Z").join("")).join("");
  // Centre: area-weighted centre of the largest part.
  const area = (r) => Math.abs(r.reduce((a, [x, y], i) => a + (x * r[(i + 1) % r.length][1] - r[(i + 1) % r.length][0] * y), 0) / 2);
  const big = rings.map((p) => p[0]).sort((a, b) => area(b) - area(a))[0];
  const cx = big.reduce((a, p) => a + p[0], 0) / big.length;
  const cy = big.reduce((a, p) => a + p[1], 0) / big.length;
  countries.push({ name, lat: +cy.toFixed(2), lon: +(((cx + 540) % 360) - 180).toFixed(2), area: Math.round(rings.reduce((a, p) => a + area(p[0]), 0)), w: +(w * s).toFixed(1), h: +(h * s).toFixed(1), d });
}
// Practice rounds in the browser: five-letter answers and the guesses Wordle accepts, and the four-letter words a
// ladder is made from. These use no random numbers, so adding them does not change the puzzles above.
const five = common.filter((w) => w.length === 5);
// Practice answers should read as plain words: no first names (propernames list) and a few places and brands the
// dictionary also holds as common nouns.
const notPlain = new Set([...read("/usr/share/dict/propernames").map((w) => w.toLowerCase()), "china", "japan", "paris", "niger", "blair", "clark", "smith", "yahoo", "adobe", "ascii", "latex", "watts", "india"]);
out("answers5.json", five.filter((w) => !notPlain.has(w)));
out("words5.json", [...new Set([...five, ...[...web2].filter((w) => w.length === 5)])].sort());
out("common4.json", four);
out("countries.json", countries.sort((a, b) => a.name.localeCompare(b.name)));
console.log({ common: common.length, bee: bee.length, valid4: valid4.length, four: four.length, ladders: ladders.length, countries: countries.length });
