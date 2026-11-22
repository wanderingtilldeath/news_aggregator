# Runbook: a reader asks to see, fix or delete their data

Who this is for: whoever answers the mailbox in `CONTACT_EMAIL` (the address on `/privacy` and `/sources`; set it on Vercel, default `digest@postpaper.in`). The promise on the privacy page is a reply within a few days. This page says how to keep it.

## Before you start

1. **Check the request comes from the address in question.** Reply only to the address the data is held under. A request from another address gets one question back: "write to us from the address you subscribed with." Never send data to an address that is not the one on file.
2. **Say which route is quickest.** Most readers do not need you:
   - Subscriber: the link at the bottom of any email opens the manage page. "Delete my data" there erases the address, the choices and the delivery records at once. "Unsubscribe instead" only stops mail.
   - Account holder: `/account` has "Delete account", which removes everything under it (below).
   - Slack or Discord channel: the stop link in any post deletes the connection.
   - Pro member: write to us (no self-serve delete, because the payment record is kept).
3. **Use a read-only query first, then the write.** Run the `SELECT`, check the row count is what you expect (one person), then run the `DELETE`. The database is Neon; use the direct connection string (no `-pooler`). Take a note of the date and what you did, without copying the personal data into the note.

## What is held, and where

| Who | Table | What is in it | Deleted by |
|---|---|---|---|
| Email subscriber | `subscribers` | address, token, choices (length, topics, rhythm, editions), `source` tag, signup / confirm / unsubscribe / bounce dates | manage page "Delete my data", or admin dashboard (Subscribers tab), or SQL below |
| Email subscriber | `deliveries` | subscriber id and edition day, nothing else | cascades from `subscribers` |
| Account holder | `accounts` | username, password hash, optional email, `email_verified_at`, linked Pro email | account page, or SQL below |
| Account holder | `account_sessions`, `account_recovery_codes`, `account_identities`, `account_sync`, `account_email_tokens` | hashes of sessions and codes, Google's `sub`, synced bookmarks / flashcards / streak / missed questions, pending confirm and reset links | cascade from `accounts` |
| Pro member | `pro_members` | email, link token, paid-until date | SQL below |
| Pro member | `pro_notes` | the notepad: saved sentences, notes, tags | cascades from `pro_members` |
| Pro member | `pro_payments`, `pro_unmatched` | payment id and the email Razorpay had; `pro_unmatched` also the amount | **kept**: the payment record (see "What we keep") |
| Slack / Discord channel | `team_hooks` | webhook address (a secret), edition, stop token, refusal count | the stop link, or SQL below |
| Admin | `admin_tokens`, `admin_audit` | the admin's email in a sign-in link or session; who did what (actor, action, target) | expire by themselves / **kept** (audit log) |
| Telegram bot users | nothing | the bot stores no chat id or message | n/a |
| Everyone who only reads | nothing | no cookie banner, no tracking cookie, no account | n/a |

Outside our database: Brevo (sends the mail and keeps its own delivery logs, under its own policy), Razorpay (holds the card or UPI details and the payer's own record), Vercel / Render / Cloudflare (request logs). We cannot delete those; say so if asked and point to the provider.

`pages`, `crawls`, `events`, `quizzes`, `intros`, `compilations` hold public news text and no reader data.

## "Show me my data" (access)

Run the queries for the person's address (lower-case it first: the columns are lower-case). Send the rows as plain text in a reply to that address. Leave out every hash and token: they are credentials, not data about the person.

```sql
-- subscriber
SELECT email, mode, frequency, tracks, topics, source, created_at, confirmed_at, unsubscribed_at, bounced_at
FROM subscribers WHERE email = 'person@example.com';

-- account (the email column is optional)
SELECT username, email, email_verified_at, created_at FROM accounts WHERE email = 'person@example.com';

-- Pro
SELECT email, until, created_at FROM pro_members WHERE email = 'person@example.com';
SELECT payment_id, created_at FROM pro_payments WHERE email = 'person@example.com';
SELECT story, title, text, note, tags FROM pro_notes WHERE email = 'person@example.com';
```

What an account has synced (bookmarks, flashcard progress, streak, missed questions) is in `account_sync`. Look it up by the account's id, and only when the person asks for it:

```sql
SELECT s.bookmarks, s.cards, s.streak, s.missed
FROM account_sync s JOIN accounts a ON a.id = s.account_id WHERE a.email = 'person@example.com';
```

## "Correct it"

- **Email subscriber**: they can change length, topics and rhythm on the manage page. A different address is a new signup plus a delete of the old one.
- **Account email**: changed on the account security page. It stays unconfirmed until they open the link we mail.
- **Pro email**: the address a payment was made with is the membership key and cannot be edited. Ask them to pay again with the right one, or grant the days to the right address from the admin dashboard (Pro tab) and revoke the wrong one.

## "Delete it" (erasure)

Do it in one transaction per person, and check the count before `COMMIT`.

```sql
BEGIN;

-- subscriber: deliveries go with the row
DELETE FROM subscribers WHERE email = 'person@example.com';           -- expect 1 (or 0)

-- account: sessions, recovery codes, Google link, synced data and pending mail links go with the row
DELETE FROM accounts WHERE email = 'person@example.com';              -- expect 1 (or 0)

-- Pro: the notepad goes with the row; accounts.pro_email is set to NULL
DELETE FROM pro_members WHERE email = 'person@example.com';           -- expect 1 (or 0)

-- Slack / Discord: they cannot give you the webhook address, so ask them to use the stop link in any post.
-- Only if they cannot, and you can tell which channel it is (the admin dashboard does not show the address):
-- DELETE FROM team_hooks WHERE id = <id>;

COMMIT;   -- or ROLLBACK; if a count is not what you expected
```

Notes:

- A subscriber who asks to be deleted and later signs up again is a new subscriber with a new token.
- Deleting the `subscribers` row also removes the "unsubscribed" marker. If the person only wants no more mail, prefer **Unsubscribe** and keep the row: otherwise a bounce, a reused list or a signup by someone else can bring mail back. Say so before you delete.
- A Pro member who is still inside a paid period loses it with the row. Tell them first; refund through Razorpay if they ask.
- An account holder with no email on file cannot be found by address. They delete the account themselves from `/account` (logged in), which needs no one from us. Do not look an account up by username on a stranger's say-so.
- The admin dashboard also removes a subscriber (Subscribers tab). It writes `subscriber.remove` to `admin_audit` with the id only, no address.

## What we keep, and why

- `pro_payments` and `pro_unmatched`: the record that a payment arrived. Payment records are kept for accounting and tax; they hold a payment id and an email, not card details. If the person insists, replace the email with a placeholder instead of deleting the row: `UPDATE pro_payments SET email = 'deleted' WHERE email = '...'`. A repeat of the same payment id from Razorpay is still recognised, so a repeated webhook cannot extend a membership twice.
- `admin_audit`: who did what in the dashboard. It holds the admin's own address, not readers'; a subscriber removal records an id.
- Unsubscribed subscribers: kept so no one is mailed after asking to stop (unless they chose "Delete my data").

## Automatic retention

- A signup that is never confirmed is deleted after 30 days (the "Prune old rows" step in `.github/workflows/crawl.yml`).
- Delivery records, channel posts, job runs, quizzes and intros older than 60 days are deleted in the same step.
- Admin sign-in links and sessions expire on their own; account sessions expire on their own.
- Confirmed subscribers, accounts and Pro members stay until they leave or ask.

## After the request

Reply to the person with what you deleted and what you kept (and why), in plain words, from `CONTACT_EMAIL`. Keep that reply; it is your record that the request was answered.

## Keeping the mailbox alive

The promise on `/privacy` depends on someone reading `CONTACT_EMAIL`. `/privacy`, `/terms`, `/contact`, `/sources` and `/pro` all point to it, and so do publishers' takedown requests. (The daily mail's footer is `MAIL_FOOTER`, a separate setting: your name and postal address.) Once a quarter, send a test message to it from an outside address and confirm it arrives. If `CONTACT_EMAIL` is not set on Vercel, the site shows `digest@postpaper.in`: check that this mailbox exists on your domain.
