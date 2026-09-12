#!/bin/sh
# Posts a fixture entry to a running itworks server.
#
# Usage: scripts/seed.sh [fixture-path]
#   fixture-path defaults to fixtures/seed-msp-sentinel.json
#   ITWORKS_URL  defaults to http://localhost:8080
set -eu

fixture="${1:-fixtures/seed-msp-sentinel.json}"
url="${ITWORKS_URL:-http://localhost:8080}"

if [ ! -f "$fixture" ]; then
	echo "seed.sh: fixture not found: $fixture" >&2
	exit 1
fi

curl -sS -X POST \
	-H "Content-Type: application/json" \
	--data-binary "@$fixture" \
	"$url/api/entries"
echo
