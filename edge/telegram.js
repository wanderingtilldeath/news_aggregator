// The Postpaper Telegram bot: commands answered from the snapshot the crawl job publishes, so it
// costs no database, no model call and no wake-up of the Render origin. Telegram posts each message
// to /telegram/webhook (see worker.js); replies() turns one update into the calls to send back.
//
//   /today [upsc|engineering]   the five most important stories
//   /topic <words>              the newest stories about those words (last 7 days)
//   /quiz [upsc|engineering]    the day's quiz as Telegram quiz polls
//   /brief [upsc|engineering]   "Today in 60 seconds"
//   /help

// The editions and their categories: a copy of internal/track and web/lib/view.ts.
export const TRACK_CATEGORIES = { upsc: ['india', 'world', 'upsc', 'editorial'], engineering: ['tech', 'ai'] };

const HELP = [
  'Postpaper, the finished daily newspaper.',
  '',
  '/today – the five most important stories',
  '/today upsc or /today engineering – for one edition',
  '/topic rbi – the newest stories about a word',
  '/quiz – today\'s quiz as polls',
  '/brief – today in 60 seconds',
].join('\n');

const esc = (s) => String(s ?? '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
const clip = (s, n) => (s.length > n ? s.slice(0, n - 1).trimEnd() + '…' : s);

// Drops the " | Site name" tail publishers add to a title, like cleanTitle in web/lib/view.ts.
const cleanTitle = (title) => {
  const head = String(title || '').split(' | ')[0].trim();
  return head.length >= 20 ? head : title;
};

const rank = (a, b) => (b.importance || 2) - (a.importance || 2) || Date.parse(b.time) - Date.parse(a.time);

function parse(text) {
  const [head, ...rest] = String(text || '').trim().split(/\s+/);
  if (!head.startsWith('/')) return { command: '', args: [] };

  // "/today@postpaper_bot" in a group chat is the same command.
  return { command: head.slice(1).split('@')[0].toLowerCase(), args: rest };
}

const trackOf = (args) => args.map((a) => a.toLowerCase()).find((a) => a in TRACK_CATEGORIES) || '';

function storyList(items, web) {
  return items
    .map((it) => {
      const link = `${web}/story/${encodeURIComponent(it.id)}`;
      const why = it.why ? `\n${esc(clip(it.why, 160))}` : '';
      return `• <a href="${esc(link)}">${esc(clip(cleanTitle(it.title || it.url), 140))}</a>${why}`;
    })
    .join('\n\n');
}

// replies returns the Telegram calls ({method, payload}) that answer an update, or none.
// deps.items() gives the snapshot's items and deps.asset(path) a published file (or null).
export async function replies(update, env, deps) {
  const message = update?.message;
  if (!message?.chat?.id || typeof message.text !== 'string') return [];

  const chat = message.chat.id;
  const web = String(env.WEB_URL || 'https://www.postpaper.in').replace(/\/+$/, '');
  const say = (text) => [{ method: 'sendMessage', payload: { chat_id: chat, text, parse_mode: 'HTML', link_preview_options: { is_disabled: true } } }];
  const { command, args } = parse(message.text);

  switch (command) {
    case 'start':
    case 'help':
      return say(esc(HELP));

    case 'today': {
      const track = trackOf(args);
      const cutoff = Date.now() - 2 * 24 * 3600 * 1000;
      const items = (await deps.items())
        .filter((it) => Date.parse(it.time) >= cutoff && (!track || TRACK_CATEGORIES[track].includes(it.category || 'other')))
        .sort(rank)
        .slice(0, 5);
      if (!items.length) return say('No stories yet for today. Try again after the next edition.');

      return say(`<b>Today${track ? ` · ${track}` : ''}</b>\n\n${storyList(items, web)}`);
    }

    case 'topic': {
      const words = args.map((a) => a.toLowerCase()).filter(Boolean);
      if (!words.length) return say('Tell me a word, for example /topic rbi');
      const items = (await deps.items())
        .filter((it) => {
          const hay = `${it.title} ${it.description} ${it.why} ${(it.tags || []).join(' ')}`.toLowerCase();
          return words.every((w) => hay.includes(w));
        })
        .sort((a, b) => Date.parse(b.time) - Date.parse(a.time))
        .slice(0, 5);
      if (!items.length) return say(`No story about "${esc(clip(words.join(' '), 60))}" in the last 7 days.`);

      return say(`<b>${esc(clip(words.join(' '), 60))}</b>\n\n${storyList(items, web)}`);
    }

    case 'quiz': {
      const quiz = await deps.asset(`/quiz/${trackOf(args) || 'upsc'}.json`);
      const questions = (quiz?.questions || []).slice(0, 5);
      if (!questions.length) return say('Today\'s quiz is not ready yet.');

      // A quiz poll shows the right answer after the reader picks; the explanation is capped at 200.
      return questions.map((q) => ({
        method: 'sendPoll',
        payload: {
          chat_id: chat,
          question: clip(q.question, 300),
          options: q.options.map((o) => clip(o, 100)),
          type: 'quiz',
          correct_option_id: q.answer,
          explanation: clip(q.explanation || '', 200),
          is_anonymous: false,
        },
      }));
    }

    case 'brief': {
      const intro = await deps.asset(`/intro/${trackOf(args) || 'upsc'}.json`);
      if (!intro?.text) return say('Today\'s briefing is not ready yet.');

      return say(`<b>Today in 60 seconds</b>\n\n${esc(intro.text)}`);
    }

    default:
      // Plain text in a private chat gets the help; in a group the bot stays quiet.
      return message.chat.type === 'private' ? say(esc(HELP)) : [];
  }
}
