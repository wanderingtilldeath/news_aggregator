import test from 'node:test';
import assert from 'node:assert/strict';
import { replies, TRACK_CATEGORIES } from './telegram.js';

const env = { WEB_URL: 'https://web.test/' };
const hoursAgo = (h) => new Date(Date.now() - h * 3600e3).toISOString();
const item = (id, o = {}) => ({ id, title: `A headline long enough to keep ${id}`, url: `https://pub.test/${id}`, category: 'india', importance: 2, time: hoursAgo(1), description: '', why: '', tags: [], ...o });
const deps = (items = [], assets = {}) => ({ items: async () => items, asset: async (path) => assets[path] ?? null });
const msg = (text, chat = { id: 7, type: 'private' }) => ({ message: { chat, text } });
const texts = (calls) => calls.map((c) => c.payload.text);

test('a track list mirrors internal/track', () => {
  assert.deepEqual(Object.keys(TRACK_CATEGORIES).sort(), ['engineering', 'upsc']);
});

test('updates that are not text messages get no reply', async () => {
  for (const update of [null, {}, { message: {} }, { message: { chat: { id: 1 } } }, { message: { chat: { id: 1 }, text: 5 } }, { edited_message: { chat: { id: 1 }, text: '/help' } }]) {
    assert.deepEqual(await replies(update, env, deps()), []);
  }
});

test('/start and /help answer with the help, escaped, to the same chat', async () => {
  for (const text of ['/start', '/help', '/HELP@postpaper_bot']) {
    const [call, ...more] = await replies(msg(text), env, deps());
    assert.equal(more.length, 0);
    assert.equal(call.method, 'sendMessage');
    assert.equal(call.payload.chat_id, 7);
    assert.match(call.payload.text, /\/today/);
    assert.equal(call.payload.parse_mode, 'HTML');
  }
});

test('/today lists the five most important stories of the last two days, best first', async () => {
  const items = [
    ...[1, 2, 3, 4, 5, 6].map((n) => item(`s${n}`, { importance: n === 6 ? 1 : 3, time: hoursAgo(n) })),
    item('old', { importance: 5, time: hoursAgo(72) }),
  ];
  const [call] = await replies(msg('/today'), env, deps(items));
  const body = call.payload.text;
  assert.match(body, /^<b>Today<\/b>/);
  assert.equal(body.match(/•/g).length, 5);
  assert.ok(!body.includes('/story/old'), 'a story older than two days must not be listed');
  assert.ok(!body.includes('/story/s6'), 'the lowest-importance story is cut');
  assert.ok(body.indexOf('/story/s1') < body.indexOf('/story/s2'), 'among equals the newest comes first');
  assert.ok(body.includes('https://web.test/story/s1'), 'links open the story on the site, with the trailing slash of WEB_URL dropped');
  assert.ok(!body.includes('pub.test'), 'never the publisher');
});

test('/today upsc and /today engineering keep their categories', async () => {
  const items = [item('a', { category: 'india' }), item('b', { category: 'tech' }), item('c', { category: 'sports' })];
  const upsc = texts(await replies(msg('/today upsc'), env, deps(items)))[0];
  assert.ok(upsc.includes('/story/a') && !upsc.includes('/story/b') && !upsc.includes('/story/c'));
  assert.match(upsc, /Today · upsc/);
  const eng = texts(await replies(msg('/today@postpaper_bot engineering'), env, deps(items)))[0];
  assert.ok(eng.includes('/story/b') && !eng.includes('/story/a'));
});

test('/today says so when there is nothing', async () => {
  const [call] = await replies(msg('/today'), env, deps([]));
  assert.match(call.payload.text, /No stories yet/);
});

test('headline and why text is HTML-escaped and clipped', async () => {
  const evil = item('x', { title: 'Fish <script>alert(1)</script> & chips for the whole of the country', why: `<b>${'w'.repeat(400)}</b>` });
  const body = texts(await replies(msg('/today'), env, deps([evil])))[0];
  assert.ok(!body.includes('<script>') && body.includes('&lt;script&gt;') && body.includes('&amp; chips'));
  assert.ok(!body.includes('<b>w'), 'the why line must not carry markup');
  assert.ok(!body.includes('w'.repeat(200)), 'the why line is clipped to 160 characters');
});

test('a title tail after " | " is dropped when the head is long enough', async () => {
  const body = texts(await replies(msg('/today'), env, deps([item('t', { title: 'A long enough headline here | The Daily Site' })])))[0];
  assert.ok(body.includes('A long enough headline here') && !body.includes('The Daily Site'));
});

test('/topic needs a word, matches every word, and says when nothing matches', async () => {
  const items = [item('a', { title: 'RBI holds the repo rate steady for now, again', tags: ['rbi'] }), item('b', { title: 'Monsoon reaches Kerala earlier than the usual date' })];
  assert.match((await replies(msg('/topic'), env, deps(items)))[0].payload.text, /Tell me a word/);
  const hit = texts(await replies(msg('/topic RBI repo'), env, deps(items)))[0];
  assert.ok(hit.includes('/story/a') && !hit.includes('/story/b'));
  const miss = texts(await replies(msg('/topic rbi monsoon'), env, deps(items)))[0];
  assert.match(miss, /No story about/);
  // What the reader typed is escaped when it is echoed back.
  assert.ok(!texts(await replies(msg('/topic <b>x</b>'), env, deps(items)))[0].includes('<b>x'));
});

test('/quiz sends up to five quiz polls with the right answer and a capped explanation', async () => {
  const q = (n) => ({ question: `Q${n}?`, options: ['a', 'b', 'c', 'd'], answer: 2, explanation: 'e'.repeat(300) });
  const calls = await replies(msg('/quiz'), env, deps([], { '/quiz/upsc.json': { questions: [1, 2, 3, 4, 5, 6, 7].map(q) } }));
  assert.equal(calls.length, 5);
  for (const c of calls) {
    assert.equal(c.method, 'sendPoll');
    assert.equal(c.payload.type, 'quiz');
    assert.equal(c.payload.correct_option_id, 2);
    assert.ok(c.payload.explanation.length <= 200);
  }
  const eng = await replies(msg('/quiz engineering'), env, deps([], { '/quiz/engineering.json': { questions: [q(1)] } }));
  assert.equal(eng.length, 1);
  assert.match((await replies(msg('/quiz'), env, deps()))[0].payload.text, /not ready/);
});

test('/brief gives the intro, escaped, or says it is not ready', async () => {
  const body = texts(await replies(msg('/brief'), env, deps([], { '/intro/upsc.json': { text: 'Rates <hold>.' } })))[0];
  assert.match(body, /Today in 60 seconds/);
  assert.ok(body.includes('Rates &lt;hold&gt;.'));
  assert.match((await replies(msg('/brief'), env, deps()))[0].payload.text, /not ready/);
});

test('plain text gets the help in a private chat and silence in a group', async () => {
  assert.equal((await replies(msg('hello'), env, deps())).length, 1);
  assert.deepEqual(await replies(msg('hello', { id: -5, type: 'supergroup' }), env, deps()), []);
  assert.deepEqual(await replies(msg('/nonsense', { id: -5, type: 'group' }), env, deps()), []);
});
