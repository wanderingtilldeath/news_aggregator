# Setup: Pro, the Telegram bot and takedowns

Everything here is optional and free to run. Do the parts you need.

## Takedown of a site

A publisher asks you to stop (the contact address is `CONTACT_EMAIL` on the web site, shown on `/sources`).

1. Add the host to the GitHub repository variable `BLOCKED_HOSTS` (comma separated, for example `www.example.com`). With or without `www.` both work.
2. Run the `crawl` workflow, or wait for the next scheduled run. It skips the site, deletes its stored pages in the "Prune old rows" step, and builds a snapshot without it, so its stories and its row on `/sources` disappear.
3. Also set `BLOCKED_HOSTS` on Render if that service runs the daily job. To remove the stories at once, run the same `DELETE` against the database by hand.

## Pro with Razorpay

Pro is paid through a Razorpay Payment Link, so no checkout code or API key is needed. It needs a Razorpay account with business verification.

1. In Razorpay, create a Payment Link for the price you want. Turn on collecting the customer's email.
2. Razorpay: Settings, Webhooks, Add. URL `https://<your Worker address>/billing/razorpay` (the Worker passes it to Render). Event: `payment_link.paid` (and `payment.captured`). Choose a secret.
3. Render environment: `RAZORPAY_WEBHOOK_SECRET` (that secret), `PRO_DAYS` (31 for a monthly price), plus the mail settings that already send confirmations.
4. Vercel environment, then redeploy: `PRO_CHECKOUT_URL` (the payment link) and `PRO_PRICE_TEXT` (for example `Rs 99 for 31 days`).
5. Test: pay with a Razorpay test-mode link. The member gets a welcome mail with a private link. Razorpay retries a webhook for a day if Render was asleep, and a repeated payment never extends twice.

A member is matched to the daily email by the email address they paid with, so they must subscribe with that address. Pro mails are sent first, have no sponsor line and link a printable edition and week.

## The Telegram bot

It runs in the Cloudflare Worker, reads the published snapshot and costs no database or model call.

1. Use the bot you already made with @BotFather (the one that posts to your channels), or create one.
2. Add two Worker secrets, from the `edge` folder: `npx wrangler secret put TELEGRAM_BOT_TOKEN` and `npx wrangler secret put TELEGRAM_WEBHOOK_SECRET` (any long random string).
3. Point Telegram at the Worker, once (use your own values):

   ```sh
   curl "https://api.telegram.org/bot<TOKEN>/setWebhook" \
     -d "url=https://<your Worker address>/telegram/webhook" \
     -d "secret_token=<the same TELEGRAM_WEBHOOK_SECRET>"
   ```

4. In @BotFather, send `/setcommands`, choose the bot and paste:

   ```text
   today - The five most important stories
   topic - Newest stories about a word, for example /topic rbi
   quiz - Today's quiz as polls
   brief - Today in 60 seconds
   help - What the bot can do
   ```

5. Message the bot `/today`. `WEB_URL` in `edge/wrangler.toml` sets the site the story links point to.

Commands work in private chats and, with `/today@yourbot`, in groups.
