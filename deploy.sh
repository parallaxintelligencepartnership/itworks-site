#!/usr/bin/env bash
# Deploys itworks.dev to pi3. Does nothing unless invoked with --go, so it
# can never be run by accident.
set -euo pipefail

if [[ "${1:-}" != "--go" ]]; then
  echo "refusing to run: pass --go to actually deploy to pi3" >&2
  exit 1
fi

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REMOTE_HOST="pi3"
REMOTE_DIR="/opt/itworks"

echo "==> syncing repo to ${REMOTE_HOST}:${REMOTE_DIR}"
rsync -az \
  --exclude '.git' \
  --exclude 'data' \
  --exclude '.vibecheck' \
  "${REPO_DIR}/" "${REMOTE_HOST}:${REMOTE_DIR}/"

echo "==> building and starting on ${REMOTE_HOST}"
ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && docker compose build --pull && docker compose up -d"

echo "==> checking health"
curl -fsS https://itworks.dev/healthz
echo
echo "==> deploy complete"
