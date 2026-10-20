// Serves the last 7 days of the digest from a snapshot that the daily crawl job
// publishes with this Worker (edge/assets, built by scripts/build-snapshot.sh).
//
//   /digest.html, /img/*, /snapshot.json  static assets, global, always warm
//   /digest, /digest/sources              answered here from snapshot.json for the
//                                         1, 2 and 7 day views
//   everything else                       proxied to the Render origin: the 30 day
//                                         view, text search, the dashboard
//
// A deploy replaces every asset at once, so the data changes only when the
// scheduler has run. The filtering below mirrors internal/api (digestQuery,
// digest, digestSources).

const DAY_MS = 24 * 60 * 60 * 1000;
const SNAPSHOT_DAYS = new Set(['1', '2', '7']);
const DEFAULT_SIZE = 24;
const MAX_SIZE = 200;

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (request.method === 'GET' && (url.pathname === '/digest' || url.pathname === '/digest/sources')) {
      const res = await fromSnapshot(url, env);
      if (res) return res;
    }

    return fetch(new Request(new URL(url.pathname + url.search, env.ORIGIN), request));
  },
};

// fromSnapshot answers a digest request, or returns null to send it to the origin.
async function fromSnapshot(url, env) {
  const query = url.searchParams;
  const days = query.get('days') ?? '1';

  // The snapshot holds articles for 7 days: other windows, text search and the
  // listing pages (articles_only=false) are the origin's job.
  if (!SNAPSHOT_DAYS.has(days) || (query.get('q') || '').trim() || query.get('articles_only') === 'false') {
    return null;
  }

  const snapshot = await loadSnapshot(url, env);
  if (!snapshot) return null;

  const since = Date.now() - Number(days) * DAY_MS;
  const inWindow = snapshot.items.filter((item) => Date.parse(item.time) >= since);

  if (url.pathname === '/digest/sources') {
    const counts = new Map();
    for (const item of inWindow) counts.set(item.source, (counts.get(item.source) || 0) + 1);

    const sources = [...counts]
      .map(([source, count]) => ({ source, count }))
      .sort((a, b) => b.count - a.count || (a.source < b.source ? -1 : a.source > b.source ? 1 : 0));

    return json({ sources });
  }

  const paging = pageParams(query);
  if (paging.error) return json({ error: paging.error }, 400);

  const source = (query.get('source') || '').trim().toLowerCase();
  const matching = source ? inWindow.filter((item) => (item.source || '').toLowerCase() === source) : inWindow;

  const { page, size } = paging;
  const total = matching.length;

  return json({
    since: new Date(since).toISOString(),
    items: matching.slice((page - 1) * size, page * size),
    page,
    page_size: size,
    total,
    total_pages: Math.ceil(total / size),
  });
}

async function loadSnapshot(url, env) {
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
