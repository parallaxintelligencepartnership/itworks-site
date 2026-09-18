# itworks - shipped 2026-09-17

## What this is
A public wall of apps that were finished through the itworks plugin's closeout, at https://itworks.build. Each entry is one JSON file under `entries/`, added by pull request; merging the pull request is the approval. Every entry gets a page and an SVG badge that shows its last audit date and goes amber after 30 days or red while a critical is open. The site is static: a Go command renders it, GitHub Actions deploys it, and nothing of ours is listening anywhere.

## How to run it
- Build: `go run ./cmd/build -out dist`, then `python3 -m http.server -d dist 8080` and open http://localhost:8080.
- Validate entries without writing anything: `go run ./cmd/build -check`.
- Render for a fixed date: `go run ./cmd/build -out dist -today 2026-01-31`. Read entries from elsewhere with `-entries <dir>`.
- Tests: `go test ./...`, plus `go vet ./...` and `gofmt -l .` before a checkpoint.
- Approval dates come from the commit that added each entry file, so the checkout needs full history. A shallow checkout is rejected by the build on purpose.

## How to deploy an update
1. Work on a branch, open a pull request; `check.yml` runs the tests and the entries-only policy.
2. Merge to `main` (squash merge; the repo allows no other method, so the approval date is the merge time and cannot be set by a submitter).
3. `pages.yml` builds and deploys automatically on the push to main, again every day at 05:17 UTC, and on demand from the Actions tab.
4. Confirm: `curl -sI https://itworks.build/ | head -1` is 200, and `curl -s https://itworks.build/api/entries.json | grep built_at` shows the new build time.
DNS records and the Pages settings are in README.md under "Pointing itworks.build at GitHub Pages".

## How to roll back
Rehearsed 2026-09-17 on the first ship tag. Ship tags are `ship-YYYY-MM-DD`; `ship-2026-09-17` is the first known-good state, and the next ship inherits it as a real rollback target.
1. `git log --oneline main` and find the last good commit or tag.
2. `git revert <bad-sha>` (a squash merge is a single commit, so a plain revert works), push main.
3. `pages.yml` redeploys on the push. If it does not run, dispatch it from the Actions tab.
4. Confirm as in the deploy section: HTTP 200 and a fresh `built_at`.
Rehearsal performed: a clean worktree at `ship-2026-09-17` was checked out, `go test ./...` and `go run ./cmd/build -out <tmp>` ran clean from it, and the worktree was removed.

## Known limitations and accepted risks
Accepted risks: none. Open findings carried into this ship, all tracked in .itworks/REVIEWS.md:
- OUTSTANDING (IMPORTANT): nothing reports a daily rebuild that stops. GitHub disables a schedule after 60 days without a commit, and a quiet wall is exactly that; badges would sit on green past day 30. Matt's decision on 2026-09-17: "the probe is needed". The fix is an n8n GitOps workflow on the estate that fetches `api/entries.json` daily and sends a Telegram alert when `built_at` is over 48 hours old or the fetch fails. It is built right after this deploy, once the live URL exists.
Limitations by design: a pending entry has no page and no badge until its pull request is merged; the badge is only as fresh as the last daily build; the site makes no model calls and stores nothing about a submitter beyond what the pull request carries.

## What breaks first and how you'd know
The daily rebuild stops (schedule disabled, workflow broken, or Pages outage). Signal: `built_at` in https://itworks.build/api/entries.json older than 48 hours, and the Telegram probe above once it exists. Second most likely: DNS or certificate trouble on the custom domain; signal is a non-200 from `curl -sI https://itworks.build/` and the Pages settings page reporting the certificate state.

## Where things live
- Code: `cmd/build` (renderer), `internal/entry` (file contract and loading), `internal/badge` (SVG and colour rules), `web/` (templates, stylesheet, fonts, icons; embedded).
- Data: `entries/*.json` in git, one file per entry. No database.
- Secrets: none. The site has no credentials of any kind; deploy uses the GitHub Actions OIDC token for Pages only.
- Backups: the git history on GitHub and the Gitea mirror at git.parallaxintelligence.xyz.
- Design record: `docs/SPEC.md`, `docs/DESIGN-NOTES.md`, `docs/design/` (including the icon rounds).
- Project state: `.itworks/` (PROJECT, DECISIONS, REVIEWS, MAP, PROFILE); audits under `docs/audit/`.

## Ship history
- 2026-09-18: the public notice board redesign (PR #1, squash merged as ccac73f), footer with the Parallax properties, canonical and Open Graph tags, JSON-LD, sitemap.xml, robots.txt, embed.js; deployed by manual dispatch because event triggers do not run (open finding)
- 2026-09-17: first ship; GitHub repo created, pages workflow run 35284262592 deployed, custom domain itworks.build on Cloudflare DNS, certificate issued, Enforce HTTPS on; freshness probe pushed to n8n GitOps (knowledge-base f1c651c)
