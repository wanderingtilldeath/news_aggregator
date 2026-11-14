# Postpaper roadmap

Postpaper is the finished daily newspaper, in your language, from sources you choose, delivered to your inbox. This file lists what is built and what comes next. Steps are built one at a time: each is tested, then waits for a go-ahead before the next starts.

Positioning (from a review of Feedly, Inoreader, Ground News, Particle, SmartNews, Flipboard, Google News and Inshorts): the market splits into RSS tools, AI brief readers and bias tools. Postpaper differs by being a finished edition rather than an endless feed, by covering Indian and regional publishers in the reader's language, by being open about sourcing, and by being email first.

## Built

| Area | What it does | Where |
|---|---|---|
| Web frontend | Next.js site on Vercel: pitch and signup above the news, UPSC and Engineering editions, standard / 1-minute / 7-minute reading, an end-of-edition marker. | `web/` |
| Subscribers | Email signup with double opt-in, choice of editions and reading length, one-click unsubscribe. | `internal/api/subscribe.go`, migrations `00009`, `00012` |
| Email | Brevo sender, confirmation mail, one edition per subscriber per day ranked by importance, with a send cap and a quiz block. | `internal/mail/`, `internal/digestmail/`, `cmd/senddigest/`, migration `00010` |
| Quality | Live blogs and boilerplate removed, Gemini summaries on by default in the crawl job, ORF, Down To Earth and Scroll added as sources. | `internal/parse/liveblog.go`, `sources.json` |
| Enrichment | A why-it-matters line, topic tags (UPSC syllabus areas), an importance score and a long summary for every recent story. | `internal/summarize/enrich.go`, migration `00011` |
| Quiz | Five questions per edition per day, on the site and in the email. | `internal/summarize/quiz.go`, migration `00013`, `web/app/quiz` |
| Archive and weekly revision | A page per past day and a printable weekly page (save as PDF from the browser). | `web/app/archive`, `web/app/week` |
| Telegram | One post per edition per day to a public channel. | `internal/telegram/`, `internal/digestmail/channels.go`, migration `00014` |
| Pilot plan | How to find 50 readers, what to ask, what to measure. | `docs/PILOT.md` |

## Next steps

Build these only after the pilot (`docs/PILOT.md`) shows readers who come back.

- **Source transparency and takedown.** A public sources page with each source's robots.txt AI policy, and a way to ask for removal honored by a blocked-hosts setting. Do this before charging anyone.
- **Language.** Store each article's language, filter by it, and write summaries in the reader's language. Hindi and other Indian languages are the biggest open gap.
- **Story grouping.** One card per event with every source that covered it.
- **Source health.** Last crawl, pages found and errors per source on the dashboard.
- **Accounts and saved articles.** Magic-link login, then save for later.
- **API keys and rate limits** for developers.
- **Billing and plans** (Stripe or Razorpay).

## Open follow-ups

- **Old static page:** `internal/api/static/index.html` is still served by the Go API and the Worker at `/`. Retire it once the Vercel site is the public address.
- **Quality of generated text:** the why-it-matters lines, importance scores and quiz questions are written by a model and nobody has reviewed them. Read a week of editions before showing them to readers.
- **Real mail check:** nothing has been sent through Brevo to a real inbox yet. Verify SPF, DKIM and DMARC pass in Gmail, and check whether Gmail shows its own unsubscribe link. It is not known whether Brevo passes the `List-Unsubscribe-Post` header through.
- **Footer:** put your name and postal address in `MAIL_FOOTER` before real subscribers get mail. Many countries require one.
- **Rate limit:** the signup limit is global (30 a minute), because the API only sees the Vercel server's address. Move to a per-client limit at the frontend or the Worker if abuse shows up.
- **Cold start:** the first signup after Render has been idle can exceed the 10 second timeout in `web/app/actions.ts`. Raise the timeout if it happens.
- **Mail quota:** Brevo's free plan is 300 mails a day, shared by confirmations and the digest. About 250 subscribers is the practical ceiling before a paid plan or another provider.
- **Hosting terms:** check that Vercel Hobby allows a commercial product before charging.
- **Token storage:** subscriber tokens are stored in plain text and never expire, because the unsubscribe link needs the raw token. Revisit with accounts in step 10.

## Setup checklist (state not verified)

Domain `postpaper.in`, Brevo domain authenticated and sender `digest@postpaper.in` verified, Render variables (`MAIL_PROVIDER`, `BREVO_API_KEY`, `MAIL_FROM`, `MAIL_FROM_NAME`, `WEB_BASE_URL`), GitHub variables and secret for the daily send (`PUBLIC_API_URL`, `MAIL_FOOTER` and the others), Vercel `API_BASE_URL`, and the custom domain pointing at Vercel (`https://www.postpaper.in` as the main address, with `WEB_BASE_URL` set to it).

- **Copyright:** Postpaper summarises other publishers' articles and links to them. Take legal advice before charging. Sources whose robots.txt disallows AI crawlers (for example Indian Express, Al Jazeera, The Guardian) are crawled today; consider dropping them for a paid product.
