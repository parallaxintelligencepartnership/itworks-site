# Lane C: Ship path — production-readiness review

Repo: /Users/matthew/parallax-private/Projects/itworks-site @ 5a87acd (read only, nothing edited)
Reviewer probes: docker build (amd64 + native), govulncheck, go build, go vet, bash -n, rsync dry run, knowledge-base grep.

Counts: 1 CRITICAL, 5 IMPORTANT, 5 ADVISORY.

---

## 1. Suspicions

### C1 (CRITICAL) The backup is not a consistent copy, never leaves pi3, and there is no restore procedure at all
- Where: `/Users/matthew/parallax-private/Projects/itworks-site/README.md:39-48`
- Condition: the database is opened WAL mode with a busy timeout (`internal/store/store.go:70`: `file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)`). The documented backup is a plain `cp /data/itworks.db /backup/...` run from a second container against the *live, running* database.
- Wrong outcome, three ways:
  1. **Inconsistent copy.** In WAL mode, committed data lives in `itworks.db-wal` until a checkpoint. `cp` of `itworks.db` alone, with the writer still running, can produce a file that is missing the most recent approvals or is torn mid-page. The `-wal` and `-shm` siblings are not copied. A restore from that file silently loses other people's submissions.
  2. **The backup does not go off the host.** The copy lands in `/opt/itworks` on pi3 itself. pi3 dying, the volume being pruned, or `docker volume rm` takes the database and the backup together. The estate already has an offsite target (`/etc/parallax/b2.env`); nothing wires itworks to it.
  3. **There is no restore path, anywhere.** Neither README.md nor docs/SPEC.md contains a single command for getting a `.db` file back into the `itworks-data` volume. Tonight, after a bad deploy, nobody has instructions.
- Why CRITICAL: PROJECT.md:25 says the stored records are *other people's* closeout summaries. Per the rubric, real user data with no working backup at ship is CRITICAL, and this qualifies twice over (the stated backup does not produce a restorable artifact, and there is no restore).
- Reproduction:
  1. `ITWORKS_DB=./data/itworks.db go run ./cmd/itworks`
  2. POST several entries with `scripts/seed.sh` while the process stays up.
  3. Without stopping the process, `cp data/itworks.db /tmp/backup.db` (the README's exact operation).
  4. `ls -l data/itworks.db-wal` — non-zero; that content is in neither the original nor the copy.
  5. `sqlite3 /tmp/backup.db 'select count(*) from entries;'` and compare with the live count. Rows committed since the last checkpoint are absent.
- What a correct version looks like: `docker run --rm -v itworks-data:/data -v "$PWD":/backup keinos/sqlite3 sqlite3 /data/itworks.db ".backup '/backup/itworks-$(date +%F).db'"` (or `VACUUM INTO`), followed by an upload to B2 using `/etc/parallax/b2.env`, plus a restore block: stop the container, `docker run --rm -v itworks-data:/data -v "$PWD":/backup alpine cp /backup/<file> /data/itworks.db`, remove stale `-wal`/`-shm`, start.

### I1 (IMPORTANT) Graceful shutdown never actually completes; in-flight POSTs are killed on every deploy and restart
- Where: `cmd/itworks/main.go:69-84`
- Condition: SIGTERM (which is exactly what `docker compose up -d` / `docker stop` sends on every redeploy).
- What happens: the shutdown goroutine calls `httpServer.Shutdown(shutdownCtx)`. `Shutdown` immediately causes the blocking `httpServer.ListenAndServe()` on line 82 to return `http.ErrServerClosed`. The error check on line 82 is false, so `main` falls straight through to its `defer`s and the process exits. The goroutine holding `Shutdown` is abandoned mid-drain. The 10 second timeout on line 74 is dead code; it never gets the chance to elapse.
- Wrong outcome: a submitter POSTing to `/api/entries` during a deploy gets the connection cut after the row may or may not have been written, and SQLite is closed out from under an in-flight write instead of after it. This is the "restart safety" item in the rubric.
- Reproduction:
  1. `ITWORKS_DB=./data/itworks.db go run ./cmd/itworks`
  2. Add a temporary `time.Sleep(3*time.Second)` at the top of the create handler (scratch copy only), POST a fixture, and send SIGTERM one second in.
  3. The process exits at once and curl reports an empty reply. With a correct shutdown it would wait for the handler to finish and return 201.
  Without a code change, the same is visible by observing that the "shutdown:" log line on line 77 can never be printed for any error, since the process is already gone.
- Fix shape: make the goroutine signal completion (a channel closed after `Shutdown` returns) and have `main` block on it after `ListenAndServe` returns `ErrServerClosed`.

### I2 (IMPORTANT) The documented rollback is not runnable; there are no tags and no previous image is kept
- Where: `README.md:50-57` (`git checkout <previous-tag>` then `./deploy.sh --go`) and `docker-compose.yml:3-4` (`build: .`, `image: itworks:local`).
- Condition: first deploy tonight, then a bad second deploy.
- Wrong outcome: (a) `git tag` in this repo returns nothing and the runbook never instructs anyone to create a tag before deploying, so `<previous-tag>` does not exist when it is needed; (b) `image: itworks:local` is overwritten in place by every `docker compose build`, so there is no prior image to fall back to — rollback is a full source rebuild on pi3 that re-pulls from the network and re-resolves the module cache, which is exactly what you do not want when the site is already broken; (c) `deploy.sh` has no `--delete`, so a rollback to an older tree leaves files the newer tree added still sitting in `/opt/itworks`.
- Reproduction: `cd /Users/matthew/parallax-private/Projects/itworks-site && git tag` → empty output. Then read README.md:52-55, which depends on that output being non-empty.
- Fix shape: tag before each deploy (`git tag deploy-$(date +%Y%m%d-%H%M)` as step 0 of the runbook), and tag the image with the commit (`image: itworks:${GIT_SHA}` plus a `itworks:previous` retag) so rollback is `docker compose up -d` against a known-good local image, with the source rebuild as the fallback rather than the only route.

### I3 (IMPORTANT) No HTTP to HTTPS redirect router; pi3's DMZ standard puts one on every public site
- Where: `docker-compose.yml:20-34` (both routers are `entrypoints=websecure` only; nothing binds the `web` entrypoint).
- Estate convention, in the knowledge base:
  - `/Users/matthew/parallax-private/Projects/claude-knowledge-base/stacks/wx-portal/docker-compose.yml:28-33` — the very stack whose secheaders labels this compose file says it copied (see `docker-compose.yml:36`). It carries a `wx-http` router on `entrypoints=web` with a `redirectscheme` middleware, commented "HTTP -> HTTPS redirect (DMZ standard)".
  - `/Users/matthew/parallax-private/Projects/claude-knowledge-base/stacks/pi3-stillpub-compose.yml:40-42` and `:48-50` — same pattern using the shared `https-redirect@file` middleware.
- Wrong outcome: `http://itworks.dev` matches no router on the `web` entrypoint, so Traefik answers 404 instead of redirecting. This service is a badge provider: README URLs and `![badge](http://itworks.dev/badge/x.svg)` typed without the scheme are exactly the traffic that lands on port 80, and it will break rather than redirect. Every other public pi3 site redirects.
- Note: this does not block certificate issuance — Traefik handles the ACME HTTP-01 challenge at the entrypoint, above the routers — so the cert will still come up.
- Reproduction (after deploy only, do not run tonight): `curl -sS -o /dev/null -w '%{http_code}\n' http://itworks.dev/` expect 301, will get 404.
- Fix: add `itworks-http` on `entrypoints=web` with `middlewares=https-redirect@file` (stillpub's form) or a local `redirectscheme` (wx-portal's form).

### I4 (IMPORTANT) Any other container on dmz-public can forge the admin header straight to port 8080 and approve or hide entries
- Where: `docker-compose.yml:13-14` (joins `dmz-infrastructure_dmz-public`) plus `docker-compose.yml:18` (`loadbalancer.server.port=8080`); the app's guard is at `internal/server/server.go:125` / `docs/SPEC.md:47`.
- Known estate precedent: `/Users/matthew/parallax-private/Projects/claude-knowledge-base/docs/superpowers/plans/2026-07-25-n8n-rotation-followups.md:367` records exactly this class — "proven 2026-07-25 that a container on `dmz-infrastructure_dmz-public` can forge `X-authentik-groups` straight to `prlx:3000` and get cross-tenant admin (Traefik bypassed)". The fix recorded there strips inbound `X-authentik-*` **at the public entrypoint**, which protects the internet path only.
- Wrong outcome: the container listens on 8080 on a shared DMZ bridge with no host port, which the runbook treats as the security boundary (`README.md:36`). It is not one for peers. A neighbour container runs `curl -X POST -H 'X-Authentik-Username: matthew' -H 'Origin: http://itworks:8080' -H 'Host: itworks:8080' http://itworks:8080/admin/entries/<id>/approve` and publishes arbitrary content to itworks.dev. The same-origin check is no obstacle because the attacker writes both headers. With `ITWORKS_TRUST_PROXY: "1"` (`docker-compose.yml:9`) the same peer forges `X-Real-Ip` and bypasses the rate limiter entirely.
- Severity note: this is container-foothold-required, not internet-reachable, which is how the estate triaged the prlx instance. It is IMPORTANT, not CRITICAL, but it is the reason the "no host port is published" check in the runbook should not be read as "only Traefik can reach it".
- Reproduction (post-deploy): from any other dmz-public container, the curl above. Pre-deploy: confirm by reading the network membership at docker-compose.yml:13-14 against the 2026-07-25 note.
- Fix shape: confirm the entrypoint-level `X-authentik-*` strip is live on pi3 before ship, and consider treating a non-Traefik source address as untrusted in the admin guard.

### I5 (IMPORTANT) The image does not build for linux/amd64 on the arm64 build host; the only path that works is building on pi3
- Where: `Dockerfile:4` and `Dockerfile:9`. The build stage has no `--platform=$BUILDPLATFORM` and the build sets no `GOARCH`, so `--platform linux/amd64` runs the whole Go toolchain under emulation.
- Probe result on this Mac (arm64, Docker running):
  `docker build --platform linux/amd64 -t itworks-audit:test .` **fails** at `RUN go mod download` with `fatal error: sweep increased allocation count` and a Go runtime stack trace — the known emulated-amd64 Go garbage collector failure.
  The same Dockerfile builds cleanly natively: `docker build -t itworks-audit:native .` succeeds.
- Both pinned digests are genuine multi-arch indexes carrying linux/amd64 (verified with `docker buildx imagetools inspect`), so the pins are correct; the defect is the build strategy, not the base images.
- Wrong outcome: the lane invariant "builds for linux/amd64 regardless of the build host" is not met. Today the ship path still works, because `deploy.sh:23` builds on pi3, which is native amd64 — so this does not block tonight. It does mean the image cannot be built or smoke-tested for the target architecture from the Mac, and it means a future switch to build-and-push would silently ship an arm64 image or fail the same way.
- Reproduction: the two `docker build` commands above, in this repo.
- Fix: `FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:... AS build` and `RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ...`. This is free here: `CGO_ENABLED=0` is already set (Dockerfile:9) and `modernc.org/sqlite v1.58.0` (go.mod:7) is pure Go with no cgo assumption anywhere, so cross-compilation is a one-line change. Also add `platform: linux/amd64` to the compose service so a stray local build is honest about its target.

### A1 (ADVISORY) Visitor IP addresses are written to the container log on every submit
- Where: `internal/server/handlers.go:134` and `:144` — `s.logger.Printf("POST /api/entries ip=%s status=%d", ip, ...)`. Mandated by `docs/SPEC.md:62` ("Log one line per POST (ip, status)").
- Tension: `.itworks/PROJECT.md:25` and `internal/store/store.go:1-2` both promise "no IP addresses are stored, ever". That promise is about the database, and the log is technically not the database — but with `restart: unless-stopped` and Docker's default json-file driver, these lines persist to disk on pi3 indefinitely, unrotated, and they are a record of who submitted what and when. A submitter reading PROJECT.md would not expect it.
- No bodies are logged anywhere, which matches the spec. The admin log line at `handlers.go:282` records the offered username, which is intended (SPEC.md:47).
- Proposed: ADVISORY, and a question for Matt below, since SPEC explicitly asks for it.
- Fix shape if he wants it: log a truncated or hashed IP, or set a `logging:` block in compose with `max-size`/`max-file` so the record ages out.

### A2 (ADVISORY) No healthcheck in compose, and the final image has no tool that could run one
- Where: `docker-compose.yml` (no `healthcheck:` key anywhere); `Dockerfile:18` is `gcr.io/distroless/static-debian12:nonroot`.
- Verified: `docker inspect itworks-audit:native --format '{{.Config.Healthcheck}}'` → `<nil>`, and an export of the image filesystem shows only `/itworks` plus empty `bin/`, `usr/bin/` — no shell, no curl, no wget.
- Consequence: `/healthz` exists and is wired (`internal/server/server.go:96`, `docs/SPEC.md:41`) and the deploy script curls it once from outside (`deploy.sh:26`), but nothing watches it afterwards. `restart: unless-stopped` only reacts to the process exiting; a hung-but-alive process stays in rotation and Traefik keeps routing to it.
- Trap to avoid: the obvious fix (`healthcheck: test: curl ...`) will fail permanently on distroless. It needs either a `HEALTHCHECK CMD ["/itworks", "-healthcheck"]` self-probe flag in the binary, or a Traefik `loadbalancer.healthcheck.path=/healthz` label, which needs no tooling in the image and is the cheaper option.
- Reproduction: the two commands above.

### A3 (ADVISORY) deploy.sh ships session state and the kickoff prompt to the DMZ host, and has no exclusion for .env files
- Where: `deploy.sh:16-20`.
- Verified by dry run (`rsync -an -v` with the script's exact excludes, to a scratch target): 57 files, including `.claude/handoff/latest.md`, `WORKFLOW-PROMPT.md`, `docs/design/*`, and `fixtures/`. The `.git`, `data`, and `.itworks` excludes do work — `data/itworks.db` is correctly held back.
- Wrong outcome: no secret is exposed today (there is no `.env` in the tree right now, confirmed). But the excludes are an allowlist by omission: `.gitignore:8-9` deliberately reserves `.env` and `.env.*`, and `deploy.sh` does not exclude them, so the first time anyone creates one for a local run it is silently rsynced to `/opt/itworks` on an internet-facing host. `.claude/handoff/latest.md` is private working notes and has no business on the DMZ box either. There is no `--delete`, so removed files linger forever.
- Reproduction: `cd <repo> && rsync -an -v --exclude '.git' --exclude 'data' --exclude '.itworks' ./ /tmp/x/ | grep -c .` → 57 files listed; note `.claude/handoff/latest.md` present and no `.env` rule in deploy.sh:17-19.
- Fix: add `--exclude '.env*' --exclude '.claude' --exclude 'WORKFLOW-PROMPT.md' --exclude 'docs'` and `--delete`. Better still, invert it: `--include` only what the build needs, since `.dockerignore` already knows that list.

### A4 (ADVISORY) Only pending rows are capped; approved and hidden rows grow without bound
- Where: `internal/server/server.go:19` (`defaultPendingCap = 500`), `internal/server/handlers.go:118-123` (503 when `CountPending() >= pendingCap`).
- Condition: the cap counts *pending* rows only. Every time Matt hides the junk, those rows leave the pending count but stay in the database, and the queue reopens for another 500.
- Disk arithmetic: the body cap is 8192 bytes (`docs/SPEC.md:52`) but only the validated fields are stored, so a maximal row is roughly name 60 + summary 160 + repo_url 200 + small ints and ISO dates, call it under 600 bytes. With the limiter at 5 POSTs per IP per hour (`docs/SPEC.md:62`), a single IP contributes about 3 KB an hour. A botnet of a thousand addresses, with Matt hiding as fast as they arrive, is roughly 3 MB an hour. That is a nuisance on pi3, not an outage, and the `unlimited public writes` item in the rubric is largely satisfied by the 500-row gate plus the per-IP limiter.
- Confirmed no paid calls: `PROJECT.md:4` ("makes no model calls, ever"), and there is no outbound HTTP client anywhere in the tree. The public POST costs nothing but disk.
- Fix shape if wanted: retain-and-purge on hidden rows older than N days, or count `pending + hidden` against the cap.

### A5 (ADVISORY) The external network is declared by key rather than by `name:`, unlike every pi3 stack
- Where: `docker-compose.yml:46-48` declares the network under the key `dmz-infrastructure_dmz-public` with `external: true` and no `name:`.
- Estate convention: `/Users/matthew/parallax-private/Projects/claude-knowledge-base/stacks/pi3-stillpub-compose.yml:54-56` and `stacks/pi3-parallax-digital-compose.yml:29-31` both use a short alias with an explicit `name: dmz-infrastructure_dmz-public`.
- Outcome: **this works** — with `external: true` and no `name:`, Compose uses the key verbatim, which is the right network — and `/Users/matthew/parallax-private/Projects/claude-knowledge-base/stacks/wx-portal/docker-compose.yml:42-44` uses the same key-only form, so there are two forms in the estate. `traefik.docker.network` (docker-compose.yml:17) is correct either way. Cosmetic only; noted so it is not mistaken for a bug later.

### Checks that passed, recorded so they are not re-opened
- The `--go` guard at `deploy.sh:6-9` is real and strict. It compares `"${1:-}"` to exactly `--go` under `set -euo pipefail`, so an unset `$1`, a different first argument, `--go` in any position other than first, or any environment variable all take the refusal branch. There is no env-var bypass and no argument-order bypass. `bash -n deploy.sh` and `bash -n scripts/seed.sh` both clean.
- `deploy.sh:12` uses the ssh alias `pi3`, never a raw IP, never root. (`README.md:32` and `:36` contain the IP `185.187.235.55`, but as DNS and post-deploy-check data, not as an ssh target.)
- Non-root and no toolchain in the final image: `docker inspect` reports `User=65532`, `Entrypoint=[/itworks]`; the exported filesystem contains the static binary, the pre-chowned `/data`, and nothing else executable. Both base images are digest-pinned, not `latest`.
- CGO consistency: `CGO_ENABLED=0` (Dockerfile:9), `modernc.org/sqlite` is pure Go, the final stage is `distroless/static` which has no libc for a cgo binary — all three agree. Nothing assumes cgo.
- Restart with an existing database is safe: the schema uses `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS` (`internal/store/store.go:92`, `:108`); there is no DROP or DELETE anywhere in the store. `restart: unless-stopped` is set (docker-compose.yml:5). The only restart defect is I1 above.
- Environment coverage: all five vars MAP.md:5 lists are read in `cmd/itworks/main.go:42-46`, each with a default. `ITWORKS_BASE_URL: https://itworks.dev` (docker-compose.yml:10) matches main.go:44 and the live domain. `ITWORKS_TRUST_PROXY: "1"` is set (docker-compose.yml:9). `ITWORKS_ADMIN_USERS: matthew` is set (docker-compose.yml:12) and an empty allowlist would deny everything (main.go:30-39). No secret value appears in the compose file or anywhere else in the tree.
- Traefik names check out against pi3: `letsencrypt` is the pi3 resolver (`claude-knowledge-base/stacks/wx-portal/docker-compose.yml:26`, `stacks/osticket/docker-compose.yml:56`, `stacks/pi3-stillpub-compose.yml:39`; the `cloudflare` resolver in the other stacks is the Tailscale-only pi1 pattern and correctly not used here). `authentik-auth@file` exists as a live pi3 middleware (`stacks/osticket/traefik-tickets-dmz.yml:32`, `system/service-dependency-map.md:1648`). Router priorities are explicit and ordered correctly (admin 10 above public 1), so `/admin` cannot fall through to the unauthenticated router.
- Dependency pinning: `go.mod` requires one direct module at an exact version with a full indirect block; `go build ./...` and `go vet ./...` are clean; `go.sum` carries the extra build-graph entries modernc pulls in, which is expected and not drift.

---

## 2. Questions for the owner

1. **Backup target.** The estate already has B2 credentials at `/etc/parallax/b2.env`. Should the itworks backup be a nightly `sqlite3 .backup` plus a B2 upload on that file, or do you want the copy to land somewhere else off pi3? This is the one item I would not ship without.
2. **IP logging.** `docs/SPEC.md:62` asks for `ip=` on every POST log line, while `PROJECT.md:25` promises no IP addresses. The spec is the newer, more specific instruction, so I have not treated it as a defect — but do you want the full address, a truncated one, or just a log rotation cap so it ages out?
3. **Rollback shape.** Do you want image tagging on pi3 (rollback = `docker compose up -d` on the previous image, seconds) or is the source-rebuild rollback acceptable given how small the site is? If the latter, the runbook still needs a "tag before you deploy" step 0, because `git tag` is currently empty.
4. **DMZ peer trust.** Is the entrypoint-level `X-authentik-*` strip from the 2026-07-25 work live on pi3 today? And do you consider a compromised dmz-public neighbour in scope for this site, or is the prlx triage (container-foothold-required, accepted) the standing answer here too?
5. **Health checking.** Traefik label (`loadbalancer.healthcheck.path=/healthz`) or a self-probe flag in the binary? The label is one line and needs nothing added to the distroless image; I would go that way unless you want compose itself to restart a hung container.
6. **Hidden rows.** Are you content to prune junk submissions by hand, or do you want hidden rows older than a set age deleted automatically?

---

## 3. Coverage

**Files read in full (no skips):**
- `Dockerfile`, `.dockerignore`, `docker-compose.yml`, `deploy.sh`, `scripts/seed.sh`
- `README.md`
- `docs/SPEC.md` (read in full; findings drawn from the Environment, Routes, admin guard, and abuse-limit sections, which are the deploy/runbook/operations material — the badge rendering and template contract sections are lane B and were read but not assessed)
- `go.mod`, `go.sum`
- `cmd/itworks/main.go`
- `.gitignore`
- `.itworks/MAP.md`, `.itworks/PROJECT.md`, `.itworks/LANES.md` (lane C section, plus the lane-independent rules)

**Files read partially, on purpose, to verify a lane C claim** (these are lane A/B's to review, not mine): `internal/store/store.go` (DSN, schema, close, absence of DROP), `internal/server/server.go` (route table, logger, pending cap), `internal/server/handlers.go` (log lines, 503 path). I did not assess their logic.

**Knowledge base:** read-only greps across `/Users/matthew/parallax-private/Projects/claude-knowledge-base/` for `authentik-auth@file`, `certresolver`, `dmz-public`, and the DMZ redirect convention; read `stacks/wx-portal/docker-compose.yml` and `stacks/pi3-stillpub-compose.yml` in full.

**Probes run:**

| Probe | Result |
|---|---|
| `bash -n deploy.sh` | clean |
| `bash -n scripts/seed.sh` | clean |
| `go build ./...` | clean |
| `go vet ./...` | clean, no output |
| `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | **`No vulnerabilities found.`** (verbatim; Go 1.26.6 darwin/arm64, govulncheck downloaded golang.org/x/vuln v1.8.0) |
| `docker info` | Docker is running |
| `docker build --platform linux/amd64 -t itworks-audit:test .` | **FAILED** at `RUN go mod download` — `fatal error: sweep increased allocation count`, Go runtime stack trace under emulation. See I5. |
| `docker build -t itworks-audit:native .` | succeeded (arm64 native), proving the Dockerfile itself is sound |
| `docker inspect itworks-audit:native` | `User=65532`, `Entrypoint=[/itworks]`, `Healthcheck=<nil>` |
| `docker export` of the built image, file listing | static binary + pre-chowned `/data` only; no shell, no curl, no toolchain |
| `docker buildx imagetools inspect` on both pinned digests | both are multi-arch indexes containing `linux/amd64`; pins are valid |
| `rsync -an -v` with deploy.sh's exact excludes, to a scratch target | 57 files; `data/itworks.db` correctly excluded; `.claude/handoff/latest.md` and `WORKFLOW-PROMPT.md` included. See A3. |
| `git tag` | empty. See I2. |

**Not run, by instruction:** no ssh to any host, no `deploy.sh --go`, no writes or edits to either repository. The scratch rsync target and the two audit images are outside both repos.
