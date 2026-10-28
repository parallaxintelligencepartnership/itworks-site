#!/usr/bin/env bash
# Checks that https://itworks.build serves exactly what this checkout builds.
#
#   bash deploy/live-check.sh [SINCE]
#
# SINCE is an ISO UTC time (2026-10-28T01:00:00Z). The check first waits, up
# to WAIT_MINUTES (default 20), until the live api/entries.json says it was
# built at or after SINCE, so a deploy that is still running is not judged on
# the old site. Then it renders this checkout for the same day the live site
# was built (-today), which makes every page byte-for-byte reproducible, and
# compares every file of that build with the live copy. The feed is compared
# without its built_at line, the one value that is a time and not a day.
# It also checks that www redirects to the apex. Exit 0 only when all match.
set -euo pipefail

SITE=${SITE:-https://itworks.build}
SINCE=${1:-1970-01-01T00:00:00Z}
WAIT_MINUTES=${WAIT_MINUTES:-20}

built_at() {
  curl -fsS --max-time 20 "$SITE/api/entries.json?nocache=$(date +%s)" |
    sed -n 's/^  "built_at": "\([0-9TZ:-]*\)",$/\1/p'
}

deadline=$(( $(date +%s) + WAIT_MINUTES * 60 ))
while :; do
  live=$(built_at || true)
  # ISO UTC times of the same shape sort as strings
  if [ -n "$live" ] && [[ ! "$live" < "$SINCE" ]]; then
    break
  fi
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "live check: the live site was built at ${live:-unknown}, not at or after $SINCE, after $WAIT_MINUTES minutes" >&2
    exit 1
  fi
  sleep 20
done
day=${live%%T*}
echo "live site built at $live; rendering this checkout for $day"

out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
go run ./cmd/build -out "$out/dist" -today "$day" >/dev/null

fail=0
checked=0
while IFS= read -r rel; do
  rel=${rel#./}
  want="$out/dist/$rel"
  got="$out/live"
  if ! curl -fsS --max-time 20 -o "$got" "$SITE/$rel"; then
    echo "MISSING $SITE/$rel" >&2
    fail=1
    continue
  fi
  if [ "$rel" = "api/entries.json" ]; then
    sed -i.bak '/^  "built_at": /d' "$got" && sed '/^  "built_at": /d' "$want" > "$out/want"
    want="$out/want"
  fi
  if ! cmp -s "$want" "$got"; then
    echo "DIFFERS $SITE/$rel" >&2
    fail=1
  fi
  checked=$((checked + 1))
done < <(cd "$out/dist" && find . -type f | sort)

www=$(curl -sS --max-time 20 -o /dev/null -w '%{http_code} %{redirect_url}' https://www.itworks.build/)
case "$www" in
  "301 https://itworks.build/") ;;
  *) echo "www.itworks.build answered '$www', not a 301 to https://itworks.build/" >&2; fail=1 ;;
esac

if [ "$fail" -ne 0 ]; then
  echo "live check FAILED ($checked files compared)" >&2
  exit 1
fi
echo "live check passed: $checked files match this checkout, www redirects"
