# The Daily Digest: web frontend

Next.js (App Router) front page for the CrawlerX API. Deploys to Vercel.

The pages fetch on the server from `API_BASE_URL`, the Cloudflare Worker in front of the Go API (`edge/worker.js`), or a local `crawlerx`. The browser never calls the API, so no CORS is needed. The 1, 2 and 7 day views are answered by the Worker from its snapshot; the 30 day view and search are proxied by the Worker to the origin.

State lives in the URL: `days`, `category`, `source`, `q`, `page`. These are the same parameters the Go API and the Worker accept.

## Run

```sh
cp .env.example .env.local     # set API_BASE_URL
npm install
npm run dev                    # http://localhost:3000
npm run build && npm run lint
```

Local API: `go run ./cmd/crawlerx` from the repo root (listens on `:8080`).

## Vercel

Set the project root directory to `web` and the environment variable `API_BASE_URL` to the Worker's URL. No other setting is needed.

## Notes

- Feed text is rendered as escaped React text and links pass `safeURL()` (`lib/api.ts`). Never use `dangerouslySetInnerHTML` with feed data.
- Article pictures are plain `<img>` pointing at the API's `/img/{id}` proxy. Do not switch to `next/image`: Vercel would fetch and re-optimize every picture.
- `Cache Components` is on. Data calls use `"use cache"` with a 5 minute `cacheLife`; the page reads `searchParams` inside `<Suspense>`.
- The old static page (`internal/api/static/index.html`) is still served by the Go API until this one matches it.
