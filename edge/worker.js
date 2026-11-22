// Serves the last 7 days of the digest from a snapshot that the daily crawl job
// publishes with this Worker (edge/assets, built by scripts/build-snapshot.sh).
//
//   /, /digest.html, /img/*, /snapshot.json  static assets, global, always warm
//                                         (/ is the news page, index.html)
//   /digest, /digest/sources              answered here from snapshot.json for the
//                                         1, 2 and 7 day views, detail=1 and one archive day
//   /story/{id}                           stories/{id}.json from the assets when the snapshot has the
//                                         story, else the origin's answer cached for six hours
//   /month                                a month's compilation from the assets (month/), else the origin
//   /cards, /months, /concepts, /entities, /ahead  one published file each (see FILES), else the origin
//   /history                              history.json from the assets (Today in history), else 404
//   /quiz, /intro                         quiz/ and intro/ files from the assets (the latest and the last
//                                         eight days); older days: the origin, cached for an hour
//   POST /telegram/webhook                the Telegram bot (telegram.js), answered from the snapshot
//   everything else                       proxied to the Render origin: the 30 day
//                                         view, text search, the dashboard
//
// A deploy replaces every asset at once, so the data changes only when the
// scheduler has run. The filtering below mirrors internal/api (digestQuery,
// digest, digestSources).

import { replies } from './telegram.js';

const DAY_MS = 24 * 60 * 60 * 1000;
const SNAPSHOT_DAYS = new Set(['1', '2', '7']);
const DEFAULT_SIZE = 24;
const MAX_SIZE = 200;
// How long Cloudflare keeps an origin answer: stories change only when the crawl runs, the quiz and
// the intro once a day.
const STORY_TTL = 6 * 3600;
const DAILY_TTL = 3600;
const STORY_ID = /^[A-Za-z0-9_-]{1,128}$/;
// The editions, in step with internal/track and the snapshot script; any other name goes to the
// origin, which answers 400.
const TRACKS = new Set(['upsc', 'engineering']);
// Paths answered by one published JSON file, and the query strings that file answers. The crawl job
// writes the files (scripts/build-snapshot.sh) and asks the API for exactly these queries.
const FILES = {
  '/cards': { file: '/cards.json', search: ['', '?days=14&limit=200'] },
  '/months': { file: '/months.json', search: [''] },
  '/concepts': { file: '/concepts.json', search: [''] },
  '/entities': { file: '/entities.json', search: ['', '?days=14&limit=150'] },
  // The dated events of the next two weeks (Week ahead page).
  '/ahead': { file: '/ahead.json', search: [''] },
};
const MONTH = /^\d{4}-(0[1-9]|1[0-2])$/;
const IST_MS = (5 * 60 + 30) * 60 * 1000;

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (request.method === 'POST' && url.pathname === '/telegram/webhook') {
      return telegramWebhook(request, url, env, ctx);
    }

    if (request.method === 'GET' && url.pathname === '/') {
      const page = await env.ASSETS.fetch(new Request(new URL('/index.html', url)));
      if (page.ok) return page;
    }

    if (request.method === 'GET' && (url.pathname === '/digest' || url.pathname === '/digest/sources')) {
      const res = await fromSnapshot(url, env);
      if (res) return res;
    }

    if (request.method === 'GET' && url.pathname.startsWith('/story/')) {
      return (await storyFromAssets(url, env)) || cachedFromOrigin(url, request, env, STORY_TTL);
    }

    if (request.method === 'GET' && FILES[url.pathname]) {
      // The published file answers the standard request; any other query is the origin's job.
      const { file, search } = FILES[url.pathname];
      return (search.includes(url.search) && (await fileFromAssets(file, url, env))) || cachedFromOrigin(url, request, env, DAILY_TTL);
    }

    // Today in history is written by the crawl job from Wikipedia; the origin has no such answer, so a missing
    // file is a 404 here, not a trip to Render.
    if (request.method === 'GET' && url.pathname === '/history') {
      return (await fileFromAssets('/history.json', url, env)) || json({ error: 'not found' }, 404);
    }

    if (request.method === 'GET' && url.pathname === '/month') {
      const track = url.searchParams.get('track') || '';
      const month = url.searchParams.get('month') || '';
      const file = TRACKS.has(track) && MONTH.test(month) ? await fileFromAssets(`/month/${track}/${month}.json`, url, env) : null;
      return file || cachedFromOrigin(url, request, env, DAILY_TTL);
    }

    if (request.method === 'GET' && (url.pathname === '/quiz' || url.pathname === '/intro')) {
      return (await dailyFromAssets(url, env)) || cachedFromOrigin(url, request, env, DAILY_TTL);
    }

    return fetch(new Request(new URL(url.pathname + url.search, env.ORIGIN), request));
  },
};

// fromSnapshot answers a digest request, or returns null to send it to the origin.
async function fromSnapshot(url, env) {
  const query = url.searchParams;
  const days = query.get('days') ?? '1';

  // The snapshot holds articles (with their long summaries) for 7 days: other windows, text search
  // and the listing pages (articles_only=false) are the origin's job. One archive day (day=) is
  // answered here when the whole day is inside the snapshot.
  const day = query.get('day');
  if ((!day && !SNAPSHOT_DAYS.has(days)) || (query.get('q') || '').trim() || query.get('articles_only') === 'false' ||
      (day && url.pathname === '/digest/sources')) {
    return null;
  }

  let since = Date.now() - Number(days) * DAY_MS;
  let until = Infinity;
  if (day) {
    // An edition day is counted in India time, like editionZone in internal/api. A malformed or
    // impossible date goes to the origin, which answers 400.
    const start = /^\d{4}-\d{2}-\d{2}$/.test(day) ? Date.parse(`${day}T00:00:00+05:30`) : NaN;
    if (Number.isNaN(start) || new Date(start + IST_MS).toISOString().slice(0, 10) !== day) return null;
    since = start;
    until = start + DAY_MS;
  }

  const snapshot = await loadSnapshot(url, env);
  if (!snapshot) return null;
  if (day && since < Date.parse(snapshot.generated_at) - (snapshot.days || 7) * DAY_MS) return null;

  const inWindow = snapshot.items.filter((item) => {
    const time = Date.parse(item.time);
    return time >= since && time < until;
  });

  if (url.pathname === '/digest/sources') {
    const counts = new Map();
    for (const item of inWindow) {
      const key = JSON.stringify([item.source, categoryOf(item)]);
      counts.set(key, (counts.get(key) || 0) + 1);
    }

    const sources = [...counts]
      .map(([key, count]) => {
        const [source, category] = JSON.parse(key);
        return { source, category, count };
      })
      .sort((a, b) => b.count - a.count || (a.source < b.source ? -1 : a.source > b.source ? 1 : 0) || (a.category < b.category ? -1 : a.category > b.category ? 1 : 0));

    return json({ sources });
  }

  const paging = pageParams(query);
  if (paging.error) return json({ error: paging.error }, 400);

  const source = (query.get('source') || '').trim().toLowerCase();
  const category = (query.get('category') || '').trim().toLowerCase();
  const matching = inWindow.filter((item) =>
    (!source || (item.source || '').toLowerCase() === source) && (!category || categoryOf(item) === category));

  const { page, size } = paging;
  const total = matching.length;

  // The long summary is part of an answer only when asked for (detail=1), like the origin does.
  const detail = query.get('detail') === '1' || query.get('detail') === 'true';
  const items = matching.slice((page - 1) * size, page * size).map((item) => (detail || !item.long ? item : { ...item, long: undefined }));

  return json({
    since: new Date(since).toISOString(),
    items,
    page,
    page_size: size,
    total,
    total_pages: Math.ceil(total / size),
  });
}

// telegramWebhook receives the bot's messages from Telegram. Telegram sends the secret chosen at
// setWebhook in a header; anything else is refused. The answer is 200 as soon as the update is
// accepted, and the replies are sent after it, because Telegram retries a slow or failed answer.
async function telegramWebhook(request, url, env, ctx) {
  if (!env.TELEGRAM_BOT_TOKEN || !env.TELEGRAM_WEBHOOK_SECRET ||
      request.headers.get('X-Telegram-Bot-Api-Secret-Token') !== env.TELEGRAM_WEBHOOK_SECRET) {
    return new Response('not found', { status: 404 });
  }

  let update;
  try {
    update = await request.json();
  } catch {
    return new Response('bad request', { status: 400 });
  }

  const deps = {
    items: async () => (await loadSnapshot(url, env))?.items ?? [],
    asset: async (path) => {
      const res = await env.ASSETS.fetch(new Request(new URL(path, url)));
      return res.ok ? res.json() : null;
    },
  };
  ctx.waitUntil(
    (async () => {
      for (const { method, payload } of await replies(update, env, deps)) {
        const res = await fetch(`https://api.telegram.org/bot${env.TELEGRAM_BOT_TOKEN}/${method}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (!res.ok) console.error(`telegram ${method} failed: ${res.status}`);
      }
    })().catch((err) => console.error('telegram webhook failed', err)),
  );

  return new Response('ok');
}

// storyFromAssets answers /story/{id} from the static copy the crawl job wrote (stories/{id}.json,
// see scripts/build-snapshot.sh), so a story read costs no origin call. It returns null for an id
// that is not in the snapshot (an older story, or a malformed id), and the caller asks the origin.
async function storyFromAssets(url, env) {
  const id = url.pathname.slice('/story/'.length);
  if (!STORY_ID.test(id)) return null;
  const file = await env.ASSETS.fetch(new Request(new URL(`/stories/${id}.json`, url)));
  if (!file.ok) return null;

  return new Response(file.body, {
    headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'public, max-age=300' },
  });
}

// fileFromAssets serves a published JSON file as an answer, or returns null when it is not there.
async function fileFromAssets(path, url, env) {
  const file = await env.ASSETS.fetch(new Request(new URL(path, url)));
  if (!file.ok) return null;

  return new Response(file.body, {
    headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'public, max-age=300' },
  });
}

// dailyFromAssets answers /quiz and /intro from the files the crawl job wrote (quiz/{track}.json
// and quiz/{track}/{day}.json, the same for intro). It returns null for anything else: an older
// day, an odd parameter, or a missing file. A missing file is not trusted to mean "none": the
// build writes nothing when its call to the API failed once, and a 404 from here would then hide
// a quiz the origin has until the next deploy. The caller asks the origin, which answers 404 itself.
async function dailyFromAssets(url, env) {
  const kind = url.pathname.slice(1);
  const track = url.searchParams.get('track') || '';
  const day = url.searchParams.get('day') || '';
  if (!TRACKS.has(track) || (day && !/^\d{4}-\d{2}-\d{2}$/.test(day))) return null;
  // The newest day not covered: the snapshot writes today and the eight days before it.
  if (day && day < new Date(Date.now() + IST_MS - 9 * DAY_MS).toISOString().slice(0, 10)) return null;

  const path = day ? `/${kind}/${track}/${day}.json` : `/${kind}/${track}.json`;
  const file = await env.ASSETS.fetch(new Request(new URL(path, url)));
  if (file.ok) {
    return new Response(file.body, {
      headers: { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'public, max-age=300' },
    });
  }

  return null;
}

// cachedFromOrigin fetches the path from the origin and lets Cloudflare cache the answer for ttl
// seconds, so a page read twice does not wake Render twice.
function cachedFromOrigin(url, request, env, ttl) {
  return fetch(new Request(new URL(url.pathname + url.search, env.ORIGIN), request), {
    cf: { cacheTtl: ttl, cacheEverything: true },
  });
}

// A page whose crawl set no category counts as "other", like the SQL does.
const categoryOf = (item) => item.category || 'other';

// The parsed snapshot is kept for the life of the isolate: the front page makes
// several /digest calls, and parsing a big snapshot each time would cost CPU. A
// deploy replaces the isolate, so a new snapshot is never served stale.
let snapshotPromise;

async function loadSnapshot(url, env) {
  snapshotPromise ??= fetchSnapshot(url, env);
  const snapshot = await snapshotPromise;
  if (!snapshot) snapshotPromise = undefined; // retry next time
  return snapshot;
}

async function fetchSnapshot(url, env) {
  try {
    const res = await env.ASSETS.fetch(new Request(new URL('/snapshot.json', url)));
    if (!res.ok) return null;

    const snapshot = await res.json();
    return Array.isArray(snapshot.items) ? snapshot : null;
  } catch {
    return null;
  }
}

function pageParams(query) {
  let page = 1;
  if (query.has('page') && query.get('page') !== '') {
    page = Number(query.get('page'));
    if (!Number.isInteger(page) || page < 1) return { error: 'page must be 1 or more' };
  }

  let size = DEFAULT_SIZE;
  for (const key of ['page_size', 'limit']) {
    if (query.has(key) && query.get(key) !== '') {
      size = Number(query.get(key));
      if (!Number.isInteger(size) || size < 1 || size > MAX_SIZE) {
        return { error: `${key} must be between 1 and ${MAX_SIZE}` };
      }
      break;
    }
  }

  return { page, size };
}

function json(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: {
      'content-type': 'application/json; charset=utf-8',
      'cache-control': status === 200 ? 'public, max-age=300' : 'no-store',
      'x-edge': 'snapshot',
    },
  });
}
