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
mkdir -p "$OUT/img" "$OUT/stories"

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
import collections
import concurrent.futures
import datetime
import json
import os
import sys
import threading
import urllib.error
import urllib.request

base, out = sys.argv[1], sys.argv[2]
# Big pictures make the first page slow and the daily deploy heavy; such a card
# shows the page's site tile instead. Override with SNAPSHOT_MAX_IMAGE_BYTES.
MAX_IMAGE = int(os.environ.get("SNAPSHOT_MAX_IMAGE_BYTES", "3000000"))
EXT = {"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif", "image/avif": "avif"}


def get(path, timeout):
    with urllib.request.urlopen(base + path, timeout=timeout) as resp:
        return resp.read(), resp.headers.get_content_type()


items, page = [], 1
while True:
    # detail=1 puts the long summaries in the snapshot (the few top stories have one), so the
    # Worker can answer the 7-minute view and archive days without the origin.
    body, _ = get(f"/digest?days=7&detail=1&page={page}&page_size=200", 60)
    data = json.loads(body)
    items += data["items"]
    if page >= data["total_pages"]:
        break
    page += 1


image_outcomes = collections.Counter()  # why a card has or lacks a picture, printed after the pass
image_failures = collections.Counter()  # (site, reason) of the pictures the proxy could not serve
outcome_lock = threading.Lock()


def outcome(kind):
    with outcome_lock:
        image_outcomes[kind] += 1


def failure(item, err):
    # The proxy says why in X-Image-Error ("upstream status 403", "too large", ...).
    reason = err.headers.get("X-Image-Error") if isinstance(err, urllib.error.HTTPError) else None
    with outcome_lock:
        image_failures[(item.get("source") or "?", reason or type(err).__name__)] += 1


def fetch_image(item):
    path = item.get("image") or ""
    if not path.startswith("/img/"):
        item["image"] = ""
        outcome("no_image")  # none named by the page, or only a shared logo (see the run's "image coverage" log line)
        return
    try:
        body, ctype = get(path, 30)
        ext = EXT[ctype]
        if len(body) > MAX_IMAGE:
            outcome("too_big")
            raise ValueError(f"{len(body)} bytes is over the {MAX_IMAGE} byte limit")
        name = path.rsplit("/", 1)[1] + "." + ext
        with open(f"{out}/img/{name}", "wb") as f:
            f.write(body)
        item["image"] = "/img/" + name
        outcome("saved")
    except Exception as err:  # a missing picture falls back to the page's site tile
        if not isinstance(err, ValueError):
            outcome("fetch_failed")
            failure(item, err)
        print(f"image skipped {path}: {err}", file=sys.stderr)
        item["image"] = ""


with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
    list(pool.map(fetch_image, items))
print("snapshot images: " + ", ".join(f"{k}={v}" for k, v in sorted(image_outcomes.items())))
for (site, reason), n in image_failures.most_common(15):
    print(f"image failures: {n:4d}  {site}  {reason}")

# One static JSON file per story, the same answer /story/{id} gives (with the exam lens and the
# "also covered by" list), so the Worker can serve a story without calling the origin. The image
# is the snapshot's own copy.
def fetch_story(item):
    try:
        body, _ = get(f"/story/{item['id']}", 30)
        story = json.loads(body)
        story["image"] = item["image"]
        with open(f"{out}/stories/{item['id']}.json", "w") as f:
            json.dump(story, f, separators=(",", ":"))
        return 1
    except Exception as err:  # the Worker falls back to the origin for this story
        print(f"story skipped {item['id']}: {err}", file=sys.stderr)
        return 0


with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
    stories = sum(pool.map(fetch_story, items))

# Everything else the site reads from the API, as static files, so the Worker answers it without the
# origin. save() writes one answer; a 404 just means there is none (a track with no quiz yet), and
# the Worker then answers 404 itself. The calls are small and independent, so they run side by side.
def save(rel, api_path, timeout=30):
    try:
        body, _ = get(api_path, timeout)
    except urllib.error.HTTPError as err:
        if err.code != 404:
            print(f"skipped {api_path}: {err}", file=sys.stderr)
        return 0
    except Exception as err:
        print(f"skipped {api_path}: {err}", file=sys.stderr)
        return 0
    os.makedirs(os.path.dirname(f"{out}/{rel}"), exist_ok=True)
    with open(f"{out}/{rel}", "wb") as f:
        f.write(body)
    return 1


IST = datetime.timezone(datetime.timedelta(hours=5, minutes=30))
today = datetime.datetime.now(IST).date()
days = [(today - datetime.timedelta(days=n)).isoformat() for n in range(9)]
TRACKS = ("upsc", "engineering")

# The quiz and the "Today in 60 seconds" intro of each track: the latest and each of the last eight
# edition days (India time); the flashcard deck; the glossary; the knowledge
# graph; and the list of monthly compilations.
jobs = [(f"{kind}/{track}.json" if day is None else f"{kind}/{track}/{day}.json",
         f"/{kind}?track={track}" + (f"&day={day}" if day else ""), 30)
        for kind in ("quiz", "intro") for track in TRACKS for day in [None] + days]
jobs += [
    ("cards.json", "/cards?days=14&limit=200", 60),
    ("months.json", "/months", 30),
    ("concepts.json", "/concepts", 30),
    ("entities.json", "/entities?days=14&limit=150", 60),
]
with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
    daily = sum(pool.map(lambda job: save(*job), jobs))

    # Each month's compilation per edition (the last twelve months).
    months = []
    if os.path.exists(f"{out}/months.json"):
        months = json.load(open(f"{out}/months.json"))["months"][:12]
    month_jobs = [(f"month/{track}/{month}.json", f"/month?month={month}&track={track}", 30) for month in months for track in TRACKS]
    daily += sum(pool.map(lambda job: save(*job), month_jobs))

snapshot = {
    "generated_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
    "days": 7,
    "items": items,
}
with open(f"{out}/snapshot.json", "w") as f:
    json.dump(snapshot, f, separators=(",", ":"))

with_images = sum(1 for i in items if i["image"])
print(f"snapshot: {len(items)} items, {with_images} images, {stories} stories, {daily} data files")
PY

# The public list of sources for the /sources page: host, sections, edition, and what the site's
# robots.txt says about AI use (the notes in sources.json). Blocked hosts are left out.
python3 - "$HERE/../sources.json" "$OUT/sources.json" <<'PY'
import json
import os
import re
import sys
from urllib.parse import urlparse

blocked = {h.strip().lower().removeprefix("www.") for h in os.environ.get("BLOCKED_HOSTS", "").split(",") if h.strip()}
by_host = {}
for src in json.load(open(sys.argv[1])):
    host = urlparse(src["seed"]).hostname or ""
    if not host or host.removeprefix("www.") in blocked:
        continue
    entry = by_host.setdefault(host, {"host": host, "names": [], "categories": [], "note": ""})
    entry["names"].append(src["name"])
    if src.get("category") and src["category"] not in entry["categories"]:
        entry["categories"].append(src["category"])
    entry["note"] = entry["note"] or src.get("note", "")
for entry in by_host.values():
    # "disallow" or an explicit =no in the robots.txt note means the site asked AI users to keep out.
    entry["policy"] = "restricts" if re.search(r"disallow|=no", entry["note"]) else "open"
with open(sys.argv[2], "w") as f:
    json.dump({"sources": sorted(by_host.values(), key=lambda e: e["host"])}, f, separators=(",", ":"))
print(f"sources: {len(by_host)} sites listed")
PY

# The news page answers at / (the Worker serves index.html) and at its old address.
cp "$HERE/../internal/api/static/index.html" "$OUT/index.html"
cp "$HERE/../internal/api/static/index.html" "$OUT/digest.html"

# Image file names carry the article id, so a name never changes its content.
cat >"$OUT/_headers" <<'HEADERS'
/img/*
  Cache-Control: public, max-age=31536000, immutable
HEADERS
