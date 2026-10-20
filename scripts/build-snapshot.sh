#!/usr/bin/env bash
# Builds the edge snapshot: the last 7 days of the digest, their images and the
# news page, as static files for the Cloudflare Worker (see edge/).
#
#   scripts/build-snapshot.sh ./crawlerx edge/assets
#
# It starts the app serve-only on loopback against $DATABASE_URL, so the snapshot
# holds exactly what the live API would return (article filter, gists, images).
set -euo pipefail

BIN=${1:?usage: build-snapshot.sh BINARY OUT_DIR}
OUT=${2:?usage: build-snapshot.sh BINARY OUT_DIR}
PORT=${SNAPSHOT_PORT:-18080}
BASE=http://127.0.0.1:$PORT
HERE=$(cd "$(dirname "$0")" && pwd)

rm -rf "$OUT"
mkdir -p "$OUT/img"

# The crawl job's own settings (SOURCES, RUN_ON_START) would start a second crawl.
env -u SOURCES -u RUN_ON_START WORKERS=0 READ_ONLY=true LISTEN_ADDR=127.0.0.1:$PORT \
  "$BIN" >"$OUT/app.log" 2>&1 &
pid=$!
trap 'kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -f "$OUT/app.log"' EXIT

for _ in $(seq 1 60); do
  curl -fs -o /dev/null "$BASE/digest.html" && break
  kill -0 "$pid" 2>/dev/null || { cat "$OUT/app.log" >&2; echo "app exited before it was ready" >&2; exit 1; }
  sleep 1
done

python3 - "$BASE" "$OUT" <<'PY'
import concurrent.futures
import datetime
import json
import os
import sys
import urllib.request

base, out = sys.argv[1], sys.argv[2]
# Big pictures make the first page slow and the daily deploy heavy; such a card
# shows the page's site tile instead. Override with SNAPSHOT_MAX_IMAGE_BYTES.
MAX_IMAGE = int(os.environ.get("SNAPSHOT_MAX_IMAGE_BYTES", "400000"))
EXT = {"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif", "image/avif": "avif"}


def get(path, timeout):
    with urllib.request.urlopen(base + path, timeout=timeout) as resp:
        return resp.read(), resp.headers.get_content_type()


items, page = [], 1
while True:
    body, _ = get(f"/digest?days=7&page={page}&page_size=200", 60)
    data = json.loads(body)
    items += data["items"]
    if page >= data["total_pages"]:
        break
    page += 1


def fetch_image(item):
    path = item.get("image") or ""
    if not path.startswith("/img/"):
        item["image"] = ""
        return
    try:
        body, ctype = get(path, 30)
        ext = EXT[ctype]
        if len(body) > MAX_IMAGE:
            raise ValueError(f"{len(body)} bytes is over the {MAX_IMAGE} byte limit")
        name = path.rsplit("/", 1)[1] + "." + ext
        with open(f"{out}/img/{name}", "wb") as f:
            f.write(body)
        item["image"] = "/img/" + name
    except Exception as err:  # a missing picture falls back to the page's site tile
        print(f"image skipped {path}: {err}", file=sys.stderr)
        item["image"] = ""


with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
    list(pool.map(fetch_image, items))

snapshot = {
    "generated_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "days": 7,
    "items": items,
}
with open(f"{out}/snapshot.json", "w") as f:
    json.dump(snapshot, f, separators=(",", ":"))

with_images = sum(1 for i in items if i["image"])
print(f"snapshot: {len(items)} items, {with_images} images")
PY

# The news page answers at / (the Worker serves index.html) and at its old address.
cp "$HERE/../internal/api/static/index.html" "$OUT/index.html"
cp "$HERE/../internal/api/static/index.html" "$OUT/digest.html"

# Image file names carry the article id, so a name never changes its content.
cat >"$OUT/_headers" <<'HEADERS'
/img/*
  Cache-Control: public, max-age=31536000, immutable
HEADERS
