# Postpaper marketing plan

This plan is for growing Postpaper's readership at close to zero cost. It is written for the owner, who has more time than money. Every number marked "target" is a proposal to aim at, not a forecast. There is no traffic history in the repository, so the first job in week 1 is to measure where things stand today.

## 1. The goal and how to know you are winning

**North-star metric: confirmed email subscribers.** The email is the only channel Postpaper owns. Social accounts, Telegram channels and search rankings can all be lost or changed by someone else. A confirmed subscriber who opens the edition each morning is the asset everything else exists to build.

Secondary metrics:

- **Confirm rate**: confirmed subscribers divided by signups. The signup is double opt-in, so an unconfirmed signup is wasted. The admin Audience tab shows both numbers.
- **Returning readers**: people who come back on a second and third week. A subscriber who never opens the email is worth little.
- **Signups by source**: the Audience tab counts signups per `utm_source` tag. This is how each channel in this plan is judged, so every link you post must carry one (section 4).
- **Pro members**: paying members (Rs 99 for 31 days, Rs 249 for 92 days). This is a later goal, not a first one (section 10).

Proposed targets for the first 90 days, to be corrected after the week 1 baseline:

| Milestone | Target | Why this number |
| --- | --- | --- |
| End of week 4 | 150 confirmed subscribers | Proves at least one channel works. The Audience tab already tracks progress to 500. |
| End of week 8 | 500 confirmed subscribers | The first milestone the dashboard already shows. |
| End of week 12 | 1,500 confirmed subscribers, 60 or more Pro-curious replies or clicks | Enough to test Pro pricing and sponsors honestly. |

A hard capacity limit sits underneath these targets. The email provider's free plan allows about 300 mails a day, and each signup costs one confirmation mail on top of the daily editions. The send job has a cap (`MaxSends`) and carries unsent mail to the next run, so nothing breaks, but readers would get the edition late. Plan the upgrade before reaching about 250 subscribers (section 9).

## 2. Who Postpaper is for, and what it promises

Postpaper is a finished daily edition: the top stories ranked, each with a plain line on why it matters, linking to the original publisher. It has a one-minute and a seven-minute read, UPSC and engineering editions, a daily quiz, flashcards, weekly revision, monthly compilations, a printable edition and a Pro notepad. The newest option is a weekly email instead of a daily one.

Three audiences, in order of fit:

1. **UPSC and state civil services aspirants.** They read the news every day on purpose, they already look for "current affairs" in search, they share notes in WhatsApp and Telegram groups, and they pay for study material. This is the best first audience because the behaviour already exists and Postpaper has features built for it (exam lens, flashcards, quiz, weekly revision, monthly compilations).
2. **Engineering students and developers.** They follow tech and AI news, read Hacker News, Reddit and X, and share links readily. They are the best audience for launches and for the "no email accounts" and "polite crawler" stories.
3. **Busy general readers.** People who want the day in a minute. This is the largest group and the hardest to reach cheaply. Treat it as a result of the other two, not a target of its own.

### The promise, in one line

"The day's news in one minute. Ranked, with why it matters. Free."

Variants by audience:

- UPSC: "Daily current affairs for UPSC, ranked by importance, with a quiz and a weekly revision. Free."
- Engineering: "The day's tech and AI news in one minute, with a line on why each story matters."
- General: "All the news, none of the noise."

### What makes it different, in terms a reader will accept

- It is finished. A reader gets a short edition, not an endless feed.
- Every story says why it matters, which saves the reader the work of judging.
- Every story links to the original publisher, so it sends readers to sources rather than replacing them.
- It is free to read, with no account and no tracking cookies.

### Objections to answer in copy, before readers raise them

| Objection | Honest answer |
| --- | --- |
| "AI summaries can be wrong." | They can. Every story links to the original, and the terms page says so. Say it first: "check the source for anything you will rely on." |
| "Another newsletter I will ignore." | Offer the weekly option, and the one-minute length. |
| "What is the catch?" | Reading is free. Pro is an optional paid notepad. No ads sold against readers' data. |
| "Is my email safe?" | Double opt-in, one-click unsubscribe, privacy page that lists what is kept. |

## 3. Messaging kit

Keep every message to one idea. Pick one line per channel and repeat it.

**Bios and short descriptions (about 150 characters):**

- Telegram channel: "Daily UPSC current affairs, ranked, with why each story matters. One minute to read. Free: postpaper.in"
- X and LinkedIn: "Postpaper: the day's news in one minute, ranked, with a line on why it matters. UPSC and engineering editions. Free."
- YouTube and Instagram: "One story, one why-it-matters, thirty seconds. Full free edition at postpaper.in"

**WhatsApp-friendly share text** (plain text, no formatting, because study groups forward it as is):

"Today's top stories, ranked, with why each matters: postpaper.in. Free, one-minute read. They also have a daily quiz."

**Rules for all copy:**

- Say "free" and "no account needed" near every call to action.
- Never claim the summaries are always correct.
- Never promise exam results. Say "helps you revise", not "helps you clear".
- Credit publishers. Every shared story link should open Postpaper's own story page, which links onward.

## 4. Measurement before anything else

Do this in week 1, before posting anywhere.

**Tag every link.** The signup form records the `utm_source` tag from the page the reader arrived on, and the admin Audience tab counts signups per source. Use short, consistent values:

| Channel | Link to post |
| --- | --- |
| Telegram UPSC channel | `https://www.postpaper.in/?utm_source=tg-upsc` |
| Telegram engineering channel | `https://www.postpaper.in/?utm_source=tg-eng` |
| Reddit | `https://www.postpaper.in/?utm_source=reddit` |
| X | `https://www.postpaper.in/?utm_source=x` |
| LinkedIn | `https://www.postpaper.in/?utm_source=linkedin` |
| YouTube and Shorts | `https://www.postpaper.in/?utm_source=yt` |
| Instagram | `https://www.postpaper.in/?utm_source=ig` |
| A named partner | `https://www.postpaper.in/?utm_source=partner-<name>` |
| Hacker News, Product Hunt | `?utm_source=hn`, `?utm_source=ph` |

Never reuse a tag for two channels, and never post an untagged link to a place you want to judge. Share buttons on the site itself carry no tag today, so shared story links cannot be told apart. That is a gap worth closing (section 6, item 4).

**Record a weekly row** in a plain spreadsheet, every Monday, from the admin Audience tab:

| Week | Signups | Confirmed | Confirm rate | Top 3 sources | Daily vs weekly | Notes on what was posted |
| --- | --- | --- | --- | --- | --- | --- |

**Decision rule:** a channel gets at least two weeks of honest effort. After that, keep it if it brings confirmed subscribers at a rate you can sustain by hand, cut it if it brings none, and move its time to the best performer.

## 5. Channels

Each channel below says why it fits, what exactly to do, how much time it takes, what it costs, how to judge it, and what can go wrong. They are ordered by expected return per hour.

### 5.1 Telegram, where the UPSC audience already is

**Why.** Postpaper already posts to two Telegram channels (UPSC and engineering). Aspirants live in Telegram, and channels are the easiest place to be discovered and to be shared onward.

**Actions.**

1. Make each channel look like a product: a clear name ("UPSC Daily Edition"), a one-line description with the website, a logo, and a pinned message that says what the channel posts and when.
2. Pin a "start here" message with the site link and the email signup link (tagged `tg-upsc` or `tg-eng`).
3. Post at a fixed time every day. People subscribe to routines. Add the quiz link once a week.
4. List both channels in Telegram channel directories (TGStat, Telemetr and similar). Listings bring steady organic discovery for no cost.
5. **Channel swaps.** Find 10 to 15 channels of similar size in UPSC, SSC, banking, engineering and coding. Message the admin with a short, specific offer: "I run a daily current-affairs channel. Want to swap a mention? I will pin yours for a day." Start with channels a little larger than yours, because a swap with a much larger channel is rarely accepted.
6. Offer admins of study channels the daily post to repost with credit and a link. This works because it saves them effort.

**Time.** 30 minutes a day for posting and replies, 2 hours a week for outreach.
**Cost.** Zero.
**Success metric.** Channel members, and signups tagged `tg-upsc` and `tg-eng`.
**Risks.** Never buy members or use growth bots. They add fake accounts, hurt reach and can get the channel restricted. Do not message people privately without being asked.

### 5.2 WhatsApp groups and the forward loop

**Why.** Study groups forward notes. A forwardable, clean message is the cheapest viral loop available.

**Actions.**

1. Add a "Share on WhatsApp" row to the edition page (done for the front page and story pages) and use the plain share text from section 3.
2. Start 2 or 3 WhatsApp groups of your own for aspirants (current affairs and doubts), capped at a manageable size, and post the edition link daily. A WhatsApp Channel (the broadcast feature) is a better fit than a group for a one-way daily post, and it needs no phone numbers shared.
3. Ask readers once a month, in the email footer or a post, to forward the edition to one study partner.

**Time.** 15 minutes a day.
**Cost.** Zero.
**Success metric.** Signups from WhatsApp-tagged links (tag a link `?utm_source=wa` in your own group posts).
**Risks.** Do not add people to groups without consent. Keep groups on topic or they are muted.

### 5.3 Search, which compounds slowly

**Why.** Aspirants search for "today's current affairs", "UPSC current affairs [date]", "monthly current affairs PDF" and "weekly current affairs" daily, in large numbers. Postpaper already produces pages that match those searches.

**Actions.**

1. Submit the sitemap in Google Search Console (the domain is already verified there for the sign-in setup). The sitemap already lists daily, weekly and monthly pages.
2. Give each recurring page a stable address that is updated rather than replaced (the weekly and monthly pages already work this way), and write a clear page title and description for each.
3. Make a free, no-login printable or PDF edition. Searches containing "pdf" are extremely common in this audience, and Pro currently gates the printable editions. Consider offering one free printable page per week as a taste of Pro.
4. Add a short "How to use Postpaper for UPSC revision" page of real, useful guidance. It brings readers who are not yet looking for a product.
5. Check Search Console each week for which queries bring impressions, and write the next page for queries with many impressions and few clicks.
6. Collect links by being useful: other study sites and blogs link to resources that are free and current.

**Time.** 1 to 2 hours a week after setup.
**Cost.** Zero.
**Success metric.** Impressions and clicks in Search Console. Expect little in the first two months; the effect builds.
**Risks.** Search results take weeks to move. Do not stuff keywords or publish pages that copy publishers' text.

### 5.4 Communities, with permission and patience

**Where.** Reddit (r/UPSC, r/developersIndia, r/IndianStudents, r/csMajors-style communities), Quora, Discord and Slack communities for aspirants and developers, college tech clubs, and Telegram discussion groups.

**How.** Be a member first. Answer questions with real help, and mention Postpaper only where it is directly useful, saying that you built it. Read each community's self-promotion rules before posting anything. A ban costs you the channel.

**Good posts to make.**

- "I built a free daily current-affairs edition for UPSC, ranked by importance. Here is how the ranking works, and what I got wrong." Sharing the build and its limits earns more trust than an ad.
- A useful free thing, such as the weekly revision sheet, offered with no sign-up required.
- Answers to "how do I stay on top of current affairs without reading five papers", where the answer is a method and Postpaper is one tool in it.

**Time.** 3 to 4 hours a week.
**Cost.** Zero.
**Success metric.** Tagged signups, and thread comments that ask questions.
**Risks.** Self-promotion rules vary and change; one removed post can be fine, repeated link-dropping is not. Never use multiple accounts, vote rings or purchased upvotes.

### 5.5 Short video and social, for reach beyond search

**Why.** Short video is where students discover study tools. The edition is already a ready supply of material.

**Actions.**

1. **Format.** One story, one why-it-matters, in 30 seconds, with the Postpaper name and site on screen at the end. Record five at a time, once a week.
2. **Carousels.** Turn the weekly revision page into an image carousel for Instagram and LinkedIn: five stories, one line each.
3. **Quote posts on X.** Post one ranked story a day with its why-it-matters line and a story page link. The story pages have social preview images, so they look good when shared.
4. **Reuse.** Cut each video once, post it to Shorts, Reels and X.
5. **Quiz clips.** A 15-second "can you answer this?" clip with the daily quiz link is cheap and draws replies.

**Time.** 3 hours a week.
**Cost.** Zero, using a phone and free editing tools.
**Success metric.** Tagged signups and, more usefully, followers who ask questions or send the post to a friend.
**Risks.** Video is slow to build. Judge it after eight weeks, not two, and do not spend money to boost it before it works organically.

### 5.6 Launch moments

One-time events that bring a burst of attention. Plan them for when the product is stable and the signup works smoothly.

| Launch | When | The angle |
| --- | --- | --- |
| Show HN on Hacker News | After the weekly digest and accounts are deployed | "A polite, resumable crawler and a no-email account system, built to run on free tiers." Developers like the engineering story. Be present to answer comments for several hours. |
| Product Hunt | Two to three weeks after the HN post | "A one-minute daily news edition, ranked, with why it matters." Prepare 3 screenshots and a one-line tagline. |
| Indie Hackers, BetaList, relevant newsletters | Within the same fortnight | The build story and the free-tier cost story. |
| r/IndianStartups and founder communities | Around the Startups page | "A free weekly funding tracker." The Startups page is a distinct product that founders and investors share. |

**Time.** About 6 hours per launch, including replies.
**Cost.** Zero.
**Success metric.** A day of high traffic and a few hundred signups at best. Launch traffic does not stay, so make the first visit count: the signup band is now on every story page and the front page.
**Risks.** The site has a free-tier backend that sleeps. Warm it before a launch, and rely on the static snapshot that Cloudflare serves for the front page, which handles spikes. Search, the 30-day view and the dashboard hit the origin and can be slow under load.

### 5.7 Partnerships, the highest trust

**Why.** A recommendation from a teacher, a YouTuber or a channel admin is worth more than any post of your own.

**Targets and offers.**

- **Coaching institutes and mentors.** Offer a free branded daily feed for their students (the Slack and Discord post feature already exists and is free), and a free Pro year for the mentor.
- **UPSC and engineering YouTubers and Telegram admins.** Offer free Pro for a year in exchange for an honest mention if they like it. Never ask for a positive review.
- **College tech clubs and placement cells.** Offer a short talk or a feed for the club's channel.
- **Newsletter swaps.** Find newsletters of similar size and trade a short mention.
- **Study-group admins.** Give the admin the daily post to forward.

**Pitch rules.** Be specific, short and free of buzzwords. Say what is in it for them (saved time, better material for their students), include one link, and ask for one small thing.

**Time.** 3 hours a week for outreach in the first month.
**Cost.** A few Pro memberships.
**Success metric.** Signups tagged `partner-<name>`.
**Risks.** Never pay for fake reviews. Disclose a free membership if a creator mentions it, as advertising rules in India expect.

### 5.8 Slack and Discord teams

**Why.** Postpaper can post the daily edition to a team's Slack or Discord channel for free. Each team that adds it exposes the edition to everyone in the channel, every day.

**Actions.** Promote it on developer and student Discord servers and in company Slack communities that allow it, with a short guide. It is a low-effort loop because the post itself carries the link.

**Success metric.** The number of connected channels, which the admin can read from the database or dashboard, and signups from team-post links (give them their own tag in the post template if possible).

### 5.9 Paid, only to test, and only after something works

Do not spend money until an organic channel has a measured cost per confirmed subscriber of zero rupees and a reason to scale. Then test, in this order:

1. A small Telegram ad or a paid post in one large UPSC channel, measured by tagged signups.
2. A small budget of search ads for a few exact queries, only if the landing page converts well organically.
3. A sponsored mention in a mid-size newsletter in the audience.

Rules for any paid test: set a hard cap in advance (section 9), run it for a fixed short period, and stop if it does not cost less per confirmed subscriber than the value you set before starting. Check current ad rates yourself; they change.

## 6. Product changes that help growth

These are small builds, ordered by value per effort. Each one makes the marketing above work better.

1. **Daily or weekly choice** (built, pushed, awaiting deployment). Lets people who dislike daily mail still join, and gives each email a one-click path to the other rhythm.
2. **Homepage pitch and share buttons** (built, awaiting deployment). Puts the promise and the signup at the top of the front page and on every story page.
3. **Subscribe prompt after a quiz or a flashcard session.** A reader who just finished a quiz is the most engaged reader you have. Show "Get tomorrow's quiz by email" at that moment.
4. **Tagged share links.** Add a `utm_source` of the sharing method (`share-wa`, `share-tg`, `share-x`) to the links behind the share buttons, so you can see which shared pages bring signups. Today shared links carry no tag.
5. **"Forward this to a friend" line in every email**, with a pre-filled WhatsApp or mail link. It turns each subscriber into a possible recruiter.
6. **Free weekly printable page.** One free printable revision page a week, with Pro keeping the full set. It targets "pdf" searches and previews Pro.
7. **A public "what we cover today" page for UPSC.** A single address that always shows today's UPSC edition, for pinning in groups.
8. **Referral count** (later). A simple "you have brought 3 readers" line, with a reward such as a free Pro week at a threshold. Do this only after the basics work, because it needs tracking and care against abuse.
9. **Embeddable "today's top 5" widget** (later) for blogs and coaching sites, with a link back.
10. **Email-welcome sequence** (later): a short second mail on day 2 that introduces the quiz and the notepad. Mind the daily mail quota first.

## 7. Twelve-week schedule

Assume about 8 to 10 hours a week. Adjust to your real time.

**Week 1: foundation.**
- Merge to main and deploy the weekly digest and homepage changes. Restart the API so migration 00034 runs.
- Record the baseline numbers from the Audience tab. Set up the weekly spreadsheet.
- Tag-plan every link (section 4). Polish both Telegram channels (name, description, pinned post, logo).
- Submit the sitemap in Search Console.

**Week 2: first reach.**
- List both channels in directories. Message 10 channels about swaps.
- Record five short videos and post them.
- Write the first Reddit post (a useful one, following the rules).

**Week 3: partnerships and search.**
- Send 10 partnership pitches (YouTubers, coaching mentors, club leads).
- Publish the "How to use Postpaper for UPSC revision" page and check Search Console.
- Add the subscribe prompt after the quiz (section 6, item 3).

**Week 4: review one.**
- Read the first four weeks of numbers. Cut anything that brought nothing. Double the effort on the best channel.
- Add tagged share links (item 4).

**Week 5 and 6: launch preparation.**
- Prepare the Show HN post and Product Hunt page. Warm the origin before launch. Pick a Tuesday to Thursday morning, India time for the audience you want (US morning for Hacker News).
- Add the "forward to a friend" line to the email (item 5).

**Week 7: launches.**
- Show HN. Reply to every comment for the first hours.
- Post the build story on Indie Hackers and in developer communities.

**Week 8: review two.**
- Compare channels again. Decide whether to unlock the Brevo upgrade based on subscriber count (section 9).
- Product Hunt launch the week after Show HN.

**Week 9 and 10: scale the winner.**
- Put most of the time into the one or two best channels. Start a small paid test only if a channel is clearly working.
- Offer Pro free for a year to 3 to 5 creators who responded well.

**Week 11: Pro test.**
- Email subscribers a short note about Pro (section 10), with a real offer. Count replies and clicks.

**Week 12: review three and plan the next quarter.**
- Compare against the targets. Write what worked, what did not, and the next 90-day plan.

## 8. Daily and weekly rhythm

**Daily, 30 to 45 minutes:**

- Check the daily edition posted correctly to email and Telegram.
- Post one story with its why-it-matters on X and one Telegram highlight.
- Answer replies and comments on every channel within a day.
- Make one helpful comment in one community.

**Weekly, 3 to 4 hours:**

- Monday: record the metrics row. Pick the week's one experiment.
- Record and schedule five short videos.
- Send outreach messages (swaps, partnerships).
- Look at Search Console and write or improve one page.

**Monthly, 2 hours:**

- Read the whole picture, cut what does not work, set next month's targets, and email subscribers a short note on what is new.

## 9. Budget and capacity

| Stage | Monthly spend | What it buys | When to unlock |
| --- | --- | --- | --- |
| 0 | Zero | Everything in this plan except paid tests | Weeks 1 to 4 |
| 1 | Roughly the price of the email upgrade | More sending capacity, so daily mail keeps going out on time | Before about 250 daily subscribers, or when the send cap starts leaving mail for the next run (check current Brevo prices; this plan does not state them) |
| 2 | A small, fixed paid-test budget | One paid Telegram post or ad | Only after one organic channel is clearly working |
| 3 | Larger paid spend | Scale the best paid channel | Only after Pro revenue or a sponsor covers it |

**Capacity watch.** Daily subscribers each cost one mail a day, and each signup costs one confirmation. Weekly subscribers cost one mail on Sunday, so all of them land on the same day. Watch the Emails tab in the admin dashboard, which compares mails today against the daily limit. The Render API also sleeps on the free plan and the first request after a sleep can time out, so warm it before a launch.

## 10. Turning readers into Pro

Do not push Pro before the free edition has real readers. A realistic order:

1. **Weeks 1 to 8: focus on free readers.** Pro is mentioned only where it fits (the Pro page, the account page).
2. **Weeks 9 to 12: ask.** Email subscribers once with a clear, honest note about what Pro adds: the synced notepad with notes, tags and search up to 10,000 notes, and printable daily and weekly editions. Link the Pro page.
3. **Offers to test, one at a time:** a free first week; a student price; the quarterly plan highlighted as the better value. Change one thing at a time and measure.
4. **Measure** clicks on the Pro page, payments, and how many members renew after the first period. Renewal is the real test of value.
5. **Sponsor line.** Once there are enough readers, the email already supports one labelled sponsor line. Sell it to a relevant coaching or study-tools brand, and keep it clearly labelled as sponsored. Pro members already get no sponsor line, which is a reason to subscribe.

## 11. Metrics, decisions and kill rules

| Question | Look at | Decision |
| --- | --- | --- |
| Is a channel worth it? | Confirmed signups by source after 2 weeks | Keep if it brings subscribers you can sustain by hand; cut if none |
| Is the signup working? | Confirm rate | Below about 50 percent: check the confirmation mail goes out, lands in inbox rather than spam, and reads clearly |
| Is the edition worth reading? | Readers returning in week 3 and 4, unsubscribe rate | If many leave in the first week, shorten the mail or push the weekly option |
| Is the pitch right? | Visitor-to-signup rate on the front page | Change the headline, one change at a time, and keep the better one |
| Is Pro ready? | Clicks to the Pro page and replies to the Pro email | If almost none, keep building the free audience |

## 12. Risks and guardrails

- **Publishers.** Growth brings attention from the sites Postpaper summarises. Keep the attribution on every story, keep the Sources and takedown page current, and answer every takedown request promptly (the policy is to remove within about a day). Honour robots.txt, as the crawler already does.
- **Accuracy.** An AI summary can mislead, and exam readers act on it. Keep the "check the source" wording visible, fix reported errors quickly and say so publicly when you do.
- **Platform rules.** Reddit, Telegram, WhatsApp and Instagram all restrict promotion. Read the rules for each, post as yourself, never use fake accounts, bought followers, scraped contact lists or bulk unsolicited messages.
- **Privacy.** Double opt-in is already in place. Do not add people to mail lists or groups without their consent, do not buy lists, and update the privacy page if a new service or data store is added.
- **Capacity.** A launch can send more mail and traffic than the free tiers carry. Warm the API first and have the email upgrade ready.
- **Burnout.** The plan relies on one person's time. Cut channels rather than stretch thin. Two channels done well beat eight done badly.

## 13. Ready-to-use copy

**Reddit post (a build-and-share post, adjust to each community's rules):**

> I built a free daily current-affairs edition for UPSC, ranked by importance, with a line on why each story matters. It takes about a minute to read. Every story links to the original publisher. The summaries are written by an AI model, so they can be wrong: please check the source for anything you will rely on. There is no account to make. I would like honest feedback on what is missing: postpaper.in

**Channel-swap message to a Telegram admin:**

> Hi, I run a daily UPSC current-affairs channel (link). I like what you post on (specific topic). Would you like to swap a mention? I can pin yours for a day and mention it in my post, and you do the same for mine. No problem at all if not.

**Creator pitch (YouTuber or mentor):**

> Hi (name), I follow your (specific video or series). I built Postpaper, a free daily current-affairs edition ranked by importance, with a quiz and weekly revision. I would like to give you a free year of Pro to try it. If you find it useful, a mention would help, and if not, I would still welcome your feedback. Here is the link: (tagged link).

**Show HN title and first comment:**

> Show HN: Postpaper, a daily news edition built on free tiers, with no-email accounts
>
> I built a crawler that reads about 40 sites politely (robots.txt, per-host delays, resumable), then writes a one-minute edition with a line on why each story matters. Accounts need no email: username, password and ten recovery codes, with optional Sign in with Google. It runs on free tiers (Render, Neon, Cloudflare, Vercel). I would like feedback on the ranking and on the account design.

**Email footer line to encourage forwarding:**

> Know someone who revises current affairs? Forward this edition, or send them postpaper.in.

**Telegram pinned post:**

> Welcome. This channel posts the day's top UPSC stories, ranked, with why each matters, every morning at (time). Free email edition and daily quiz: postpaper.in

## 14. The first seven days, as a checklist

- [ ] Merge the latest branch to main so the weekly choice and homepage changes are live, and restart the API for migration 00034.
- [ ] Open the admin Audience tab and write down today's numbers as the baseline.
- [ ] Create the weekly spreadsheet from section 4.
- [ ] Prepare tagged links for each channel.
- [ ] Polish both Telegram channels: name, description, logo, pinned message.
- [ ] Submit the sitemap in Search Console.
- [ ] List both channels in two Telegram directories.
- [ ] Message 10 channels about swaps.
- [ ] Record five short videos.
- [ ] Draft the first Reddit post and read the community's rules.
- [ ] Decide the Pro offer to test in week 11, and note it.
- [ ] Confirm the confirmation mail arrives in the inbox, not spam, from a fresh address.

## 15. What is assumed and what to verify

- There is no traffic or subscriber data in the repository. All targets above are proposals until week 1 gives the baseline.
- The email provider's free daily limit is taken from the dashboard's own comment (about 300 a day). Check the provider's current plans and prices before relying on them.
- Advertising rates, platform rules and directory features change. Check each before spending time or money.
- India's advertising and data-protection rules apply to promotion, endorsements and mailing lists. This plan assumes consent-based mail and disclosed free memberships, and is not legal advice.
