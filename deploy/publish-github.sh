#!/usr/bin/env bash
# Publishes this checkout's main to the GitHub repo, where pages.yml builds
# and deploys itworks.build to GitHub Pages on the push (the live site, its
# domain and its Pages settings stay as they are).
#
#   GITHUB_TOKEN=... bash deploy/publish-github.sh
#
# Gitea is where the team works; GitHub is where the site is published and
# where outside entries arrive as pull requests. The push is a fast-forward
# only: when GitHub main has commits this checkout lacks (an entry merged on
# GitHub), nothing is pushed and the script says how to bring them over.
# Prints the push time (ISO UTC) as the last line, for deploy/live-check.sh;
# when GitHub already has this commit it prints the epoch instead.
# The token is read from the environment only (a Drone secret), never from a
# file, and is handed to git through GIT_ASKPASS, never on a command line.
set -euo pipefail

REPO=parallaxintelligencepartnership/itworks-site
URL=https://github.com/$REPO.git

if [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "publish: the Drone secret github_itworks_token is not set, so nothing can reach GitHub" >&2
  exit 1
fi

head=$(git rev-parse HEAD)
git fetch --quiet --no-tags "$URL" +refs/heads/main:refs/remotes/github/main
remote=$(git rev-parse refs/remotes/github/main)

if [ "$remote" = "$head" ]; then
  echo "publish: GitHub main is already $head"
  echo "1970-01-01T00:00:00Z"
  exit 0
fi
if ! git merge-base --is-ancestor "$remote" "$head"; then
  echo "publish: GitHub main has commits that Gitea main lacks (an entry merged on GitHub):" >&2
  git log --oneline "$head..$remote" >&2
  echo "Bring them into Gitea main with a pull request: git fetch $URL main, merge FETCH_HEAD into the branch, open the pull request. The next push to main publishes." >&2
  exit 1
fi

askpass=$(mktemp)
trap 'rm -f "$askpass"' EXIT
printf '#!/bin/sh\nprintf "%%s\\n" "$GITHUB_TOKEN"\n' > "$askpass"
chmod 700 "$askpass"

since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_ASKPASS=$askpass GIT_TERMINAL_PROMPT=0 \
  git push --quiet "https://x-access-token@github.com/$REPO.git" "$head:refs/heads/main"
echo "publish: pushed $head to GitHub main ($(git rev-list --count "$remote..$head") commits)"
echo "$since"
