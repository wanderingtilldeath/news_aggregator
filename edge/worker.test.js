import test from 'node:test';
import assert from 'node:assert/strict';
import worker from './worker.js';

const SECRET = 's3cret-value';
const hook = (headers = {}, body = JSON.stringify({ message: { chat: { id: 1, type: 'private' }, text: '/help' } })) =>
  new Request('https://edge.test/telegram/webhook', { method: 'POST', headers: { 'Content-Type': 'application/json', ...headers }, body });
const assets = { fetch: async () => new Response('missing', { status: 404 }) };
const ctxOf = () => {
  const jobs = [];
  return { jobs, waitUntil: (p) => jobs.push(p) };
};

// Every call to Telegram goes through fetch: record it instead of reaching the network.
function withTelegram(t) {
  const calls = [];
  const real = globalThis.fetch;
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), body: JSON.parse(init.body) });
    return new Response('{}', { status: 200 });
  };
  t.after(() => {
    globalThis.fetch = real;
  });
  return calls;
}

test('the webhook answers 404 unless the bot token, the secret and the header all agree', async (t) => {
  const calls = withTelegram(t);
  const full = { TELEGRAM_BOT_TOKEN: 'tok', TELEGRAM_WEBHOOK_SECRET: SECRET, ASSETS: assets };
  const cases = {
    'no secret configured': [{ TELEGRAM_BOT_TOKEN: 'tok', ASSETS: assets }, { 'X-Telegram-Bot-Api-Secret-Token': SECRET }],
    'no bot token configured': [{ TELEGRAM_WEBHOOK_SECRET: SECRET, ASSETS: assets }, { 'X-Telegram-Bot-Api-Secret-Token': SECRET }],
    'no header': [full, {}],
    'wrong header': [full, { 'X-Telegram-Bot-Api-Secret-Token': 'nope' }],
    'a prefix of the secret': [full, { 'X-Telegram-Bot-Api-Secret-Token': SECRET.slice(0, 5) }],
    'the secret with more on the end': [full, { 'X-Telegram-Bot-Api-Secret-Token': SECRET + 'x' }],
    'empty header with an empty secret configured': [{ TELEGRAM_BOT_TOKEN: 'tok', TELEGRAM_WEBHOOK_SECRET: '', ASSETS: assets }, { 'X-Telegram-Bot-Api-Secret-Token': '' }],
  };
  for (const [name, [env, headers]] of Object.entries(cases)) {
    const ctx = ctxOf();
    const res = await worker.fetch(hook(headers), env, ctx);
    assert.equal(res.status, 404, name);
    assert.equal(ctx.jobs.length, 0, `${name}: nothing may be scheduled`);
  }
  assert.equal(calls.length, 0, 'a refused call must never reach Telegram');
});

test('with the right secret the update is accepted at once and answered after', async (t) => {
  const calls = withTelegram(t);
  const ctx = ctxOf();
  const res = await worker.fetch(hook({ 'X-Telegram-Bot-Api-Secret-Token': SECRET }), { TELEGRAM_BOT_TOKEN: 'tok', TELEGRAM_WEBHOOK_SECRET: SECRET, ASSETS: assets }, ctx);
  assert.equal(res.status, 200);
  await Promise.all(ctx.jobs);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, 'https://api.telegram.org/bottok/sendMessage');
  assert.equal(calls[0].body.chat_id, 1);
});

test('a bad body with the right secret is a 400, and a Telegram failure does not break the answer', async (t) => {
  withTelegram(t);
  const env = { TELEGRAM_BOT_TOKEN: 'tok', TELEGRAM_WEBHOOK_SECRET: SECRET, ASSETS: assets };
  const headers = { 'X-Telegram-Bot-Api-Secret-Token': SECRET };
  assert.equal((await worker.fetch(hook(headers, '{not json'), env, ctxOf())).status, 400);

  globalThis.fetch = async () => {
    throw new Error('network down');
  };
  const ctx = ctxOf();
  const quiet = console.error;
  console.error = () => {};
  t.after(() => {
    console.error = quiet;
  });
  assert.equal((await worker.fetch(hook(headers), env, ctx)).status, 200);
  await Promise.all(ctx.jobs); // rejects would fail the test: the worker must swallow and log
});

test('a GET to the webhook path is not the bot', async () => {
  const env = { TELEGRAM_BOT_TOKEN: 'tok', TELEGRAM_WEBHOOK_SECRET: SECRET, ASSETS: assets, ORIGIN: 'https://origin.invalid' };
  const real = globalThis.fetch;
  let proxied = '';
  globalThis.fetch = async (req) => {
    proxied = String(req.url ?? req);
    return new Response('origin', { status: 200 });
  };
  try {
    await worker.fetch(new Request('https://edge.test/telegram/webhook', { method: 'GET', headers: { 'X-Telegram-Bot-Api-Secret-Token': SECRET } }), env, ctxOf());
  } finally {
    globalThis.fetch = real;
  }
  assert.ok(!proxied.includes('api.telegram.org'));
});

// The published files the crawl job writes are served from the assets without calling the origin.
function assetsWith(files) {
  return {
    fetch: async (req) => {
      const body = files[new URL(req.url).pathname];
      return body === undefined ? new Response('missing', { status: 404 }) : new Response(JSON.stringify(body), { status: 200 });
    },
  };
}

test('/ahead is served from ahead.json for the standard request and goes to the origin for any other', async (t) => {
  const calls = [];
  const real = globalThis.fetch;
  globalThis.fetch = async (req) => {
    calls.push(String(req.url ?? req));
    return new Response(JSON.stringify({ from: 'origin' }), { status: 200 });
  };
  t.after(() => {
    globalThis.fetch = real;
  });
  const env = { ASSETS: assetsWith({ '/ahead.json': { items: [{ id: 'a' }] } }), ORIGIN: 'https://origin.test' };

  const res = await worker.fetch(new Request('https://edge.test/ahead'), env, ctxOf());
  assert.equal(res.status, 200);
  assert.deepEqual(await res.json(), { items: [{ id: 'a' }] });
  assert.equal(calls.length, 0, 'the standard request must not reach the origin');

  await worker.fetch(new Request('https://edge.test/ahead?x=1'), env, ctxOf());
  assert.ok(calls.some((c) => c.startsWith('https://origin.test/ahead')), 'another query is the origin\'s job');
});

test('/history is the published file, or a 404 that never calls the origin', async (t) => {
  const real = globalThis.fetch;
  let originCalls = 0;
  globalThis.fetch = async () => {
    originCalls++;
    return new Response('origin', { status: 200 });
  };
  t.after(() => {
    globalThis.fetch = real;
  });

  const withFile = { ASSETS: assetsWith({ '/history.json': { date: '2026-11-22', items: [{ year: 1995, text: 'Toy Story', url: 'https://en.wikipedia.org/wiki/Toy_Story' }] } }), ORIGIN: 'https://origin.test' };
  const ok = await worker.fetch(new Request('https://edge.test/history'), withFile, ctxOf());
  assert.equal(ok.status, 200);
  assert.equal((await ok.json()).items[0].year, 1995);

  const none = await worker.fetch(new Request('https://edge.test/history'), { ASSETS: assetsWith({}), ORIGIN: 'https://origin.test' }, ctxOf());
  assert.equal(none.status, 404);
  assert.equal(originCalls, 0, 'a missing history file must not wake the origin');
});

test('the cron pings the origin health page and never throws, even when the origin is down', async (t) => {
  const real = globalThis.fetch;
  const seen = [];
  globalThis.fetch = async (url) => {
    seen.push(String(url));
    throw new Error('origin down');
  };
  t.after(() => {
    globalThis.fetch = real;
  });
  const ctx = ctxOf();
  await worker.scheduled({}, { ORIGIN: 'https://origin.test' }, ctx);
  await Promise.all(ctx.jobs);
  assert.deepEqual(seen, ['https://origin.test/digest.html']);
});
