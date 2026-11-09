#!/usr/bin/env bash
# Publishes this checkout's main to the GitHub repo, where pages.yml builds
# and deploys itworks.build to GitHub Pages on the push (the live site, its
# domain and its Pages settings stay as they are).
#
#   GITHUB_DEPLOY_KEY="$(cat key)" bash deploy/publish-github.sh
#
# Gitea is where the team works; GitHub is where the site is published and
# where outside entries arrive as pull requests. The push is a fast-forward
# only: when GitHub main has commits this checkout lacks (an entry merged on
# GitHub), nothing is pushed and the script says how to bring them over.
# Prints the push time (ISO UTC) as the last line, for deploy/live-check.sh;
# when GitHub already has this commit it prints the epoch instead.
# The push uses an SSH deploy key (write access to the GitHub repo only). The
# key is read from the environment only (the Drone secret
# github_itworks_deploy_key), written to a 600 temp file that is removed on
# exit, and checked against GitHub's published SSH host keys (pinned below).
# GITHUB_TOKEN (Drone secret github_itworks_token) remains as a fallback.
set -euo pipefail

REPO=parallaxintelligencepartnership/itworks-site
URL=https://github.com/$REPO.git
PUSH_URL=git@github.com:$REPO.git

if [ -z "${GITHUB_DEPLOY_KEY:-}" ] && [ -z "${GITHUB_TOKEN:-}" ]; then
  echo "publish: the Drone secret github_itworks_deploy_key is not set, so nothing can reach GitHub" >&2
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

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

since=$(date -u +%Y-%m-%dT%H:%M:%SZ)
if [ -n "${GITHUB_DEPLOY_KEY:-}" ]; then
  # GitHub's published SSH host keys (https://api.github.com/meta)
  cat > "$tmp/known_hosts" <<'KEYS'
github.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl
github.com ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTYAAAAIbmlzdHAyNTYAAABBBEmKSENjQEezOmxkZMy7opKgwFB9nkt5YRrYMjNuG5N87uRgg6CLrbo5wAdT/y6v0mKV0U2w0WZ2YB/++Tpockg=
github.com ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCj7ndNxQowgcQnjshcLrqPEiiphnt+VTTvDP6mHBL9j1aNUkY4Ue1gvwnGLVlOhGeYrnZaMgRK6+PKCUXaDbC7qtbW8gIkhL7aGCsOr/C56SJMy/BCZfxd1nWzAOxSDPgVsmerOBYfNqltV9/hWCqBywINIR+5dIg6JTJ72pcEpEjcYgXkE2YEFXV1JHnsKgbLWNlhScqb2UmyRkQyytRLtL+38TGxkxCflmO+5Z8CSSNY7GidjMIZ7Q4zMjA2n1nGrlTDkzwDCsw+wqFPGQA179cnfGWOWRVruj16z6XyvxvjJwbz0wQZ75XK5tKSb7FNyeIEs4TT4jk+S4dhPeAUC5y+bDYirYgM4GC7uEnztnZyaVWQ7B381AK4Qdrwt51ZqExKbQpTUNn+EjqoTwvqNj4kqx5QUCI0ThS/YkOxJCXmPUWZbhjpCg56i+2aB6CmK2JGhn57K5mj0MNdBXA4/WnwH6XoPWJzK5Nyu2zB3nAZp+S5hpQs+p1vN1/wsjk=
KEYS
  (umask 077; printf '%s\n' "$GITHUB_DEPLOY_KEY" > "$tmp/key")
  GIT_SSH_COMMAND="ssh -i $tmp/key -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes -o UserKnownHostsFile=$tmp/known_hosts" \
    git push --quiet "$PUSH_URL" "$head:refs/heads/main"
else
  printf '#!/bin/sh\nprintf "%%s\\n" "$GITHUB_TOKEN"\n' > "$tmp/askpass"
  chmod 700 "$tmp/askpass"
  GIT_ASKPASS=$tmp/askpass GIT_TERMINAL_PROMPT=0 \
    git push --quiet "https://x-access-token@github.com/$REPO.git" "$head:refs/heads/main"
fi
echo "publish: pushed $head to GitHub main ($(git rev-list --count "$remote..$head") commits)"
echo "$since"
