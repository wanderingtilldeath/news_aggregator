# Postpaper pilot plan

Goal: find out whether people want Postpaper enough to come back every day and, later, pay for it. Do this before building billing or accounts. Two weeks, about 50 readers, one audience at a time.

## Before the pilot

1. Fill `MAIL_FOOTER` with your name and postal address, and set the Brevo and Telegram settings from `.env.example`.
2. Create the two public Telegram channels (UPSC and Engineering), make the bot an administrator, and set `TELEGRAM_CHAT_UPSC` and `TELEGRAM_CHAT_ENGINEERING`.
3. Check the first week of editions yourself. Read the "why it matters" lines and the quiz questions for mistakes. The model writes them and nobody has reviewed them.
4. Send yourself `senddigest -to you@example.com` and check SPF, DKIM and DMARC pass in Gmail and that the unsubscribe link works.

## Finding readers

- **UPSC edition:** aspirant groups on Telegram, Reddit r/UPSC, WhatsApp study groups, coaching-institute alumni you know. Ask 20 people for a 15-minute call first.
- **Engineering edition:** your own team, colleagues, a post on LinkedIn or Hacker News "Show HN".
- Tell them it is free during the pilot and ask for feedback by replying to the email.

## Questions for the calls

1. What do you read every day today, and what do you pay for it?
2. What does a good day of current affairs (or engineering news) look like? How long does it take?
3. Show the 1-minute and 7-minute editions. Which would you read? What is missing?
4. Would you take the quiz? Would you print the weekly revision?
5. What would you pay per month for this, and what would make you cancel?

## What to measure

Run these against the production database.

```sql
-- confirmed readers, by edition choice and reading length
SELECT tracks, mode, count(*) FROM subscribers
WHERE confirmed_at IS NOT NULL AND unsubscribed_at IS NULL GROUP BY tracks, mode ORDER BY 3 DESC;

-- signups that never confirmed (a confirmation mail problem or a weak pitch)
SELECT count(*) FROM subscribers WHERE confirmed_at IS NULL;

-- mails sent per day
SELECT edition_day, count(*) FROM deliveries GROUP BY 1 ORDER BY 1 DESC LIMIT 14;

-- unsubscribes
SELECT count(*) FROM subscribers WHERE unsubscribed_at IS NOT NULL;
```

Postpaper does not track opens or clicks. Use replies, the number of readers who stay subscribed, and the calls. If you want open rates, Brevo's dashboard shows them for the mails it sends.

## Decision after two weeks

- At least 30 readers still subscribed, a handful of unprompted replies, and several people who say they would pay: build accounts and billing (roadmap steps 10 to 12).
- Readers but no replies: improve story quality and the quiz first.
- Few readers: change the audience or the promise before building anything else.
