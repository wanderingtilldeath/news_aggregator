// Edge cache in front of the serve-only web service.
//
// The digest changes once a day, but the origin sleeps (Render free) and so does
// its database (Neon free). This Worker answers from Cloudflare's cache and
// refreshes entries in the background, so visitors do not wait for either.
//
//   fresh   (< FRESH_SECONDS old)  served from cache
//   stale   (< KEEP_S old)    served from cache, refreshed after the response
//   missing                   fetched from the origin and stored
//
// A background refresh gets about 30 s after the response; a cold origin can take
// longer, but the request still wakes it, so the next refresh succeeds.

const FRESH_SECONDS = 60 * 60; // override with the FRESH_SECONDS var
const KEEP_S = 24 * 60 * 60;
const BROWSER_S = 300;

const CACHEABLE = [/^\/digest\.html$/, /^\/digest(\/sources)?$/, /^\/img\/[^/]+$/];

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);

    if (request.method !== 'GET' || !CACHEABLE.some((re) => re.test(url.pathname))) {
      return fetch(new Request(origin(env, url), request));
    }

    const cache = caches.default;
    const key = new Request(url.toString(), { method: 'GET' });

    const hit = await cache.match(key);
    if (hit) {
      const age = Date.now() - Number(hit.headers.get('x-stored-at') || 0);
      const stale = age > Number(env.FRESH_SECONDS || FRESH_SECONDS) * 1000;
      if (stale) ctx.waitUntil(refresh(cache, key, env, url).catch(() => {}));
      return served(hit, stale ? 'STALE' : 'HIT');
    }

    const res = await refresh(cache, key, env, url);
    return served(res, 'MISS');
  },
};

function origin(env, url) {
  return new URL(url.pathname + url.search, env.ORIGIN).toString();
}

// refresh fetches the origin and stores a successful response. Failures are not
// cached, so an old entry survives an origin outage.
async function refresh(cache, key, env, url) {
  const res = await fetch(origin(env, url), { headers: { accept: key.headers.get('accept') || '*/*' } });
  if (!res.ok) return res;

  const stored = new Response(res.body, res);
  stored.headers.set('cache-control', `public, max-age=${KEEP_S}`);
  stored.headers.set('x-stored-at', String(Date.now()));
  await cache.put(key, stored.clone());
  return stored;
}

function served(res, state) {
  const out = new Response(res.body, res);
  out.headers.set('x-edge', state);
  if (res.ok) out.headers.set('cache-control', `public, max-age=${BROWSER_S}`);
  return out;
}
