# Project: itworks

## What it is
A wall of apps that were built with the vibecheck plugin and finished through its closeout, plus a README badge per app showing the audit date and open critical count. Precisely: a single Go binary serving a public HTML wall, a JSON submit endpoint, SVG badges, and an admin approval page, backed by one SQLite file. The site makes no model calls, ever.

## Who uses it
the public - auth needed: yes, for the admin approval route only (Authentik forward-auth at Traefik); roles needed: yes: visitor (read wall, badges, entry pages), publisher (unauthenticated POST from a vibecheck closeout, entry hidden until approved), admin (Matt, approve or hide entries)

## Terminology map
| Your words | Real term |
|---|---|
| "the wall" | the public list of approved closeout summaries at /wall |
| "badge" | SVG served at /badge/{id}.svg, colored by audit age and open critical count |
| "closeout summary" | the JSON payload the closeout skill POSTs: name, one line, source label, repo link, audit tier and date, found/fixed/accepted, critical open |
| "approved" | entry status that makes it visible on the wall and entry page |
| "stale" | audit date 30 or more days old (amber badge) |

## Stack
Go 1.26, standard library HTTP and html/template, static assets via embed
modernc.org/sqlite (pure Go, pinned exact), one SQLite file on a Docker volume
No Node build step, no accounts, no email, no LLM calls
Hosting target: pi3 (x86_64), docker compose under /opt/itworks, Traefik letsencrypt route, admin route behind authentik-auth forward-auth

## Data
Real data: other people's closeout summaries (display name, one line, source label, optional repo URL, tier, date, four counts) | Sample data: fixtures/seed-msp-sentinel.json, POSTed by scripts/seed.sh | Sensitive: no by design; the payload never carries code, paths, secrets, or environment, and no IP addresses are stored

## Where it will live
internet - exposure notes: https://itworks.dev via Traefik only, no host port published, /admin behind Authentik forward-auth; deploy and domain purchase deferred by Matt on 2026-09-11 - paid services: none

## Definition of done
1. go test ./..., go vet ./..., govulncheck all clean.
2. Local run: seed entry POSTed, approved, visible on the wall; badge green; amber when audit date is 40 days back; red when one critical is open.
3. Plugin closeout asks the opt in publish question; publish.md lists exactly what is and is not sent; lint and the closeout harness case pass.
4. Dockerfile, compose, Traefik labels, deploy.sh, and runbook staged; nothing touches pi3 until Matt says go.
5. vibecheck lint clean on this repo; /vibecheck:checkpoint run before the report.

## Verification expectations
| Feature | Success proof | Failure behavior |
|---|---|---|
| POST /api/entries | 201 with id, badge_url, entry_url; row status pending | 400 on bad field or unknown field, 413 over 8 KB, 415 wrong content type, 429 over per IP limit, 503 when pending queue is full |
| GET /api/entries and /wall | approved entries listed newest first | pending and hidden entries never appear |
| GET /badge/{id}.svg | valid SVG, green/amber/red per rules, Cache-Control set | 404 for unknown or hidden id |
| GET /e/{id} | approved entry page renders | 404 for pending, hidden, unknown |
| Admin approve/hide | status flips, wall updates | 403 without the forward-auth header; 403 on cross origin POST; 404 unknown id |
| Rate limit | 6th POST from one IP inside an hour gets 429 with Retry-After | limiter state is per process; restart resets it (accepted) |
| Badge color rules | unit tests for the 29/30 day boundary and critical precedence | none |
