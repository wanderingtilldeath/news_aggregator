#!/usr/bin/env bash
# Visits the public pages after a deploy so the caches are filled before a reader arrives: the
# crawl job pays any cold start, not a reader. Vercel serves a stale page and refreshes it in the
# background on the first visit, so the pages are visited twice, a little apart.
#
#   scripts/warm-caches.sh https://www.postpaper.in https://news-digest-edge.example.workers.dev
#
# It never fails the job: a page that does not answer is reported and skipped.
set -uo pipefail

WEB=${1:?usage: warm-caches.sh WEB_URL [API_URL]}
API=${2:-}
WEB=${WEB%/}
API=${API%/}

paths=(/ "/?track=upsc" "/?track=engineering" /quiz "/quiz?track=engineering" /archive /week /rss.xml /sitemap.xml)
for category in india world upsc tech ai; do
  paths+=("/?category=$category")
done
for n in 0 1 2; do
  paths+=("/archive/$(TZ=Asia/Kolkata date -d "$n day ago" +%F 2>/dev/null || TZ=Asia/Kolkata date -v-"$n"d +%F)")
done

# The newest stories, the ones readers open first.
if [ -n "$API" ]; then
  ids=$(curl -fsS -m 30 "$API/digest?days=1&page_size=15" 2>/dev/null |
    python3 -c 'import json,sys; print(" ".join(i["id"] for i in json.load(sys.stdin)["items"]))' 2>/dev/null || true)
  for id in $ids; do
    paths+=("/story/$id")
  done
fi

for pass in 1 2; do
  echo "pass $pass: ${#paths[@]} pages"
  printf '%s\n' "${paths[@]}" | xargs -P 4 -I{} curl -s -o /dev/null -m 60 -w "%{http_code} %{time_total}s {}\n" "$WEB{}" | sort | tee "/tmp/warm-$$.txt" | awk '$1 != 200' || true
  echo "$(grep -c '^200 ' "/tmp/warm-$$.txt") of ${#paths[@]} answered 200"
  [ "$pass" = 1 ] && sleep "${WARM_PAUSE:-15}"
done
rm -f "/tmp/warm-$$.txt"
exit 0
