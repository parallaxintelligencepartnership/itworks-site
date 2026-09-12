Use a workflow. Tonight's job: build the public wall and badge service for the vibecheck plugin at itworks.dev and seed it with one entry. You are the PM: write specs, delegate to Sonnet implementers in parallel, review every result against the spec. Read ~/parallax-private/Projects/claude-knowledge-base/CLAUDE.md first. Run /vibecheck:kickoff in this session before launching the workflow; the wall is built under the plugin and that is part of the story.

WHAT THIS IS
itworks.dev is a wall of apps that were built with the vibecheck plugin and finished through its closeout. Each entry is a closeout summary: display name, one line on what it does, source label (public or closed) with an optional repo link, audit tier and date, findings found / fixed / accepted, and a badge. The badge is an SVG for READMEs showing the audit date and the open critical count. It is green when the audit is under 30 days old with zero critical open, amber when the audit is stale, red when a critical is open. The badge can look bad on purpose; that is the honest part. The site makes NO model calls, ever. Visitors' own Claude Code sessions do the work; this site only stores summaries and serves badges.

DECIDED, DO NOT REOPEN
- Domain itworks.dev, DNS at the registrar points at pi3. The plugin keeps the name vibecheck. The site never uses the phrase "vibe check" anywhere.
- Stack: one Go binary, standard library plus modernc.org/sqlite pinned, SQLite file on a volume, static assets embedded with embed. No Node build step. No accounts, no signups, no email.
- Host: pi3, docker compose under /opt/itworks, Traefik letsencrypt route in the existing pi3 pattern, the global security-headers middleware, behind CrowdSec like the other public sites. The admin approval route sits behind the existing Authentik forward-auth on pi3.
- Publish is opt-in from the vibecheck closeout skill. Closeout asks: "Publish this closeout summary to itworks.dev? (default no)". Yes sends one JSON POST with the summary fields above and nothing else: no code, no file paths, no secrets, no environment. The response returns an entry id and a badge URL. New entries are hidden until approved in the admin route.
- Abuse limits in the service: 8 KB body cap, per-IP rate limit, strict field validation and length caps, entries unlisted until approved.
- Repo ~/parallax-private/Projects/itworks with origin Gitea ParallaxIntelligence/itworks.

DELIVERABLES, one Sonnet implementer per lane, lanes run in parallel
1. api: Go service. Routes: POST /api/entries, GET /api/entries (approved only), GET /badge/{id}.svg, GET /e/{id} entry page, GET /admin and POST /admin/entries/{id}/approve and /hide. Tests cover validation, the rate limit, and the badge color rules.
2. web: landing page and wall page as embedded templates and one CSS file. Copy in plain English, US spelling, no dashes of any kind inside UI strings. Motion is future-forward but GPU-only (transform and opacity), and honors prefers-reduced-motion. The landing page shows all three badge states with the line that the badge can look bad on purpose. Install instructions for the plugin are two commands.
3. plugin: in ~/parallax-private/Projects/vibecheck add the opt-in publish question to skills/project-closeout, a references/publish.md that lists exactly what is sent and what is not, and a README section. Version stays 0.1.4; a content change means uninstall and reinstall to refresh. Do not touch gates or the wording of any other step. Run scripts/vibecheck-lint.sh and the closeout case in tests/pressure/harness.
4. deploy: Dockerfile (multi-stage, pinned base image digests, run uname -m on pi3 before choosing the image), compose file, Traefik labels copied from an existing pi3 public site, deploy.sh, and a runbook section in README. Nothing published on a host port; Traefik only.
5. seed: from ~/parallax-private/Projects/ccs-sentinel/.vibecheck (read only) write the first entry as "MSP Sentinel", closed source, using the 2026-09-07 audit numbers. Do not name the employer or any client. Deliver it as a fixture the api lane can POST.

VERIFY PHASE after the lanes finish
- go test ./... and go vet ./... pass, govulncheck clean.
- Local run: POST the seed entry, approve it, confirm the wall shows it, confirm the badge renders green, set the audit date 40 days back and confirm amber, set one critical open and confirm red.
- vibecheck lint clean on both the itworks repo and the plugin repo.
- After deploy: curl the pi3 public IP on the container port and confirm it is closed; curl https://itworks.dev/badge/{seed}.svg and confirm the SVG.
- Run /vibecheck:checkpoint on the itworks repo before reporting.

RULES
- No credentials in any file. No LLM API calls from the site, ever.
- Pin dependencies exact and commit the lockfile.
- Atomic commits, one concern each, no Co-Authored-By trailers.
- Implementers report under 300 words with file paths and evidence. If a spec is ambiguous, the implementer stops and asks; it does not guess.
- Deploying to pi3 and adding the Traefik route: WAIT FOR MY GO. Build, test and stage everything, then stop and ask before any command touches pi3.

FINAL REPORT to me: what is live and its URL, what is not, the seed entry link and badge, and the exact commands left for me.
