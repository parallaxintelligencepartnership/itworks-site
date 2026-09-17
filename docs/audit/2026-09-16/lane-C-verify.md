# Lane C verification — Ship path, production readiness

Verifier: independent of the lane C author. Repo `/Users/matthew/parallax-private/Projects/itworks-site` @ 5a87acd, read only.
Scratch: `.../scratchpad/audit/` (walprobe, shutdown, xbuild). No ssh, no `deploy.sh --go`, no repo writes.

---

## C1 — Backup: WAL copy hazard, off-host path, restore procedure
**Verdict: PARTIAL (the hazard is real and worse than reported; the "no backup off host" leg is REFUTED).**
**Final severity: IMPORTANT** (not CRITICAL).

### Reproduction: the README `cp` produces a database with no data at all
Built `go build -o scratch/itworks ./cmd/itworks`, ran against a scratch db:

```
ITWORKS_DB=<scratch>/walprobe/verifyC.db ITWORKS_ADDR=:18093 ITWORKS_BASE_URL=http://localhost:18093 ./itworks
POST 1..5 /api/entries (fixtures/seed-msp-sentinel.json)  ->  201 201 201 201 201
```

Files while the process is still running:

```
-rw-r--r--   4096 verifyC.db
-rw-r--r--  32768 verifyC.db-shm
-rw-r--r--  82432 verifyC.db-wal
```

Counts:

| Source | `select count(*) from entries` |
|---|---|
| live db (read-only handle, WAL visible) | **5** |
| `cp verifyC.db backup-readme.db` (README's exact operation, process still running) | **0 — `Parse error: no such table: entries`** |
| `VACUUM INTO 'backup-vacuum.db'` on the live db | **5** |
| live file after the process stopped (WAL checkpointed on close) | 5 |

The main `.db` file is 4096 bytes — one header page. The schema *and* every row are still in the 82 KB `-wal`, which the README command does not copy. The backup is not "missing recent rows"; it has **no tables**. Confirmed, and more severe than the report's wording.

### Leg 2 of the report ("the backup never leaves pi3") — REFUTED
`claude-knowledge-base/stacks/kopia/DR-COVERAGE.md:16`:
> `| /var/lib/docker/volumes | all | kopia source | Docker named volumes (Postgres, Vault, Wazuh, Gitea, etc.) |`

and `stacks/kopia/STATE.md:24`:
> `| PI3 | pi3 | 03:30 ET | Standalone — Mailcow, Authentik, public sites |`

The `itworks-data` named volume lives under `/var/lib/docker/volumes`, which is in pi3's nightly kopia source set to B2 (`parallax-fleet`, prefix `pi3/`, credentials `/etc/parallax/b2.env`, `.claude/rules/kopia-dr.md:45`). Real user data therefore **does** have a working off-host backup at ship. The rubric's CRITICAL condition ("real user data with no working backup at ship") is not met, so this drops to IMPORTANT: an undocumented/non-runnable operational procedure with no restore path.

Residual risk kopia does not remove: kopia snapshots the live `.db`, `-wal` and `-shm` as ordinary files, non-atomically. That is usually recoverable (unlike the README `cp`, which is not), but the estate already has the right answer for this class — a pre-backup logical dump, `DR-COVERAGE.md:19`:
> `| Postgres + MySQL logical dumps | all | pre-backup hook → /opt/backups/db-dumps/ | Belt-and-suspenders DB recovery |`

### Fix spec
- **File:** `README.md` "## Backup" section (lines 39-48) — replace the `cp` command.
  Correct command for this stack (final image is distroless, no `sqlite3`, no shell — so run sqlite from a throwaway container, not `docker exec`):
  ```
  docker run --rm -v itworks-data:/data -v /opt/backups/db-dumps:/backup keinos/sqlite3 \
    sqlite3 /data/itworks.db ".backup '/backup/itworks-$(date +%F).db'"
  ```
  (`VACUUM INTO '/backup/itworks-$(date +%F).db'` is equivalent and was the form proven above.)
- **Off-host path:** none to invent. `/opt` is already a kopia source (`DR-COVERAGE.md:13`), so writing the dump to `/opt/backups/db-dumps/` on pi3 puts it in the 03:30 ET run to B2 with no new credentials and no new B2 wiring. Match the existing pre-backup-hook pattern rather than adding a bespoke b2 upload.
- **Restore block (must be added, README, new "## Restore" section):** stop the container (`docker compose stop itworks`); `docker run --rm -v itworks-data:/data -v /opt/backups/db-dumps:/backup alpine sh -c 'rm -f /data/itworks.db-wal /data/itworks.db-shm && cp /backup/<file> /data/itworks.db && chown 65532:65532 /data/itworks.db'`; `docker compose up -d`; verify `curl -fsS https://itworks.dev/healthz` and that the wall shows the expected entries. Deleting the stale `-wal`/`-shm` is required — a restored `.db` next to an old WAL is corruption.
- **Check that fails before / passes after:** with the app running and rows posted, take a backup by the documented command and open the copy: `sqlite3 <copy> 'select count(*) from entries;'` must equal the live count. Today it errors with `no such table: entries`.
- **Must not change:** volume name `itworks-data`; `deploy.sh`'s `--go` guard; the `pi3` ssh alias; the amd64 base pins.

---

## I1 — Graceful shutdown never completes
**Verdict: CONFIRMED. Final severity: IMPORTANT** (rubric: "a restart that loses in-flight work").

`cmd/itworks/main.go:72-85`, exact lines:

```go
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()

	log.Printf("itworks.dev listening on %s (...)", ...)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}
```

Nothing joins the goroutine. `Shutdown` makes `ListenAndServe` return `ErrServerClosed` immediately; `main` returns, runs `defer db.Close()`, and the process exits while `Shutdown` is still draining.

### Reproduction (`scratchpad/audit/shutdown/main.go`, this wiring copied verbatim with a 3 s handler)
SIGTERM sent 1 s into a 3 s request:

```
18:16:26 listening
18:16:27 handler: start
18:16:28 main: ListenAndServe returned, main exiting      <- 1s in
process GONE at ~120ms after SIGTERM
curl: curl_status=000 exit=52  (52) Empty reply from server
```

`handler: done, writing 201` and `shutdown: RETURNED (drain complete)` were never printed. The 10 s timeout is dead code.

Fixed variant (`shutdown/fixed/`) — same file plus `done := make(chan struct{})`, `defer close(done)` in the goroutine, `<-done` after `ListenAndServe` returns:

```
18:16:43 listening
18:16:44 handler: start
18:16:47 handler: done, writing 201     <- full 3s honoured
FIXED curl_status=201
```

### Fix spec
- **File/function:** `cmd/itworks/main.go`, `main()`. Add `done := make(chan struct{})`; `defer close(done)` as the first line of the shutdown goroutine; after the `ListenAndServe` error check, block on `<-done` before `main` returns (so `defer db.Close()` runs after the drain).
- **Check:** a test or probe that starts the server, holds a request open, sends SIGTERM, and asserts the in-flight request completes with its real status. Fails today (empty reply, exit 52), passes after (201).
- **Must not change:** the 10 s timeout value, `signal.NotifyContext` set (`os.Interrupt`, `syscall.SIGTERM`), `ReadHeaderTimeout`, `defer db.Close()` ordering relative to the drain.

---

## I2 — Rollback is not runnable
**Verdict: CONFIRMED. Final severity: IMPORTANT** (rubric: undocumented or non-runnable rollback).

Evidence:
- `git tag` in the repo → **empty output** (no tags at all). `README.md:50-57` rollback begins `git checkout <previous-tag>`.
- `docker-compose.yml:3-4`:
  ```
      build: .
      image: itworks:local
  ```
  A single mutable tag. `deploy.sh:23` runs `docker compose build --pull && docker compose up -d` on pi3, overwriting `itworks:local` in place. No previous image survives; rollback is a full source rebuild on the DMZ host at the worst moment.
- `deploy.sh:16-20` has no `--delete`, so a rollback to an older tree leaves newer files behind in `/opt/itworks`.

### Fix spec (smallest workable)
- **File:** `README.md` runbook — add step 0: `git tag deploy-$(date +%Y%m%d-%H%M) && git push --tags` (or just a local tag) **before** each `./deploy.sh --go`, so `<previous-tag>` exists when needed.
- **File:** `docker-compose.yml:4` — `image: itworks:${GIT_SHA:-local}`, and in `deploy.sh` export `GIT_SHA="$(git rev-parse --short HEAD)"` for the remote build, plus a `docker tag itworks:$GIT_SHA itworks:previous` before the new build. Rollback then is `GIT_SHA=<old> docker compose up -d` on pi3 — seconds, no network.
- **Check:** `git tag` non-empty before deploy; on pi3 `docker images itworks` shows at least two distinct tags after the second deploy. Both fail today.
- **Must not change:** `deploy.sh`'s `--go` guard (`deploy.sh:6-9`), `REMOTE_HOST="pi3"` alias, `REMOTE_DIR=/opt/itworks`, the `itworks-data` volume, the digest-pinned amd64 bases.

---

## I3 — No HTTP→HTTPS redirect router
**Verdict: CONFIRMED. Final severity: IMPORTANT** (rubric: a missing redirect the estate standard requires).

`docker-compose.yml:22` and `:31` bind `entrypoints=websecure` only; nothing binds `web`.

Estate standard, quoted:
- `claude-knowledge-base/stacks/wx-portal/docker-compose.yml:28-33` — the stack this compose file says it copied (`docker-compose.yml:36`):
  ```
        # HTTP -> HTTPS redirect (DMZ standard)
        - "traefik.http.routers.wx-http.rule=Host(`wx.parallaxintelligence.online`)"
        - "traefik.http.routers.wx-http.entrypoints=web"
        - "traefik.http.routers.wx-http.middlewares=wx-https-redirect"
        - "traefik.http.middlewares.wx-https-redirect.redirectscheme.scheme=https"
        - "traefik.http.middlewares.wx-https-redirect.redirectscheme.permanent=true"
  ```
- `claude-knowledge-base/stacks/pi3-stillpub-compose.yml:40-42`:
  ```
        traefik.http.routers.stillpub-http.rule: Host(`stillpub.com`) || Host(`www.stillpub.com`)
        traefik.http.routers.stillpub-http.entrypoints: web
        traefik.http.routers.stillpub-http.middlewares: https-redirect@file
  ```
  and the same shape again at `:48-50` for `stillpub.app`.

Is a global entrypoint redirect present on pi3 that would make this unnecessary? **No evidence of one, and positive evidence against.**
- `grep -rn "redirections"` across the whole knowledge base returns exactly one hit, and it is an unrelated Ansible comment (`projects/ccs-devbox/roles/pve_patch/tasks/main.yml:277`). The only `entrypoints.web.http.redirections` flags anywhere are in `archive/configs/traefik-swarm-stack-secrets.yml:19-20` — the archived **pi1 swarm** Traefik, not pi3.
- `session-notes/2026-07-26-dmz-security-closeout.md:338-339`: "The DMZ Traefik has **no static config file**. Everything is CLI flags in `/root/dmz-infrastructure/docker-compose.yml`". The only entrypoint-level flag recorded for pi3 in that note (`:175`) is `--entrypoints.websecure.http.middlewares=...` — websecure, no `web` redirection.
- Decisive circumstantially: stillpub and wx-portal would not each carry a per-router `web` redirect if the entrypoint already did it.

(Not ssh-verifiable here by instruction. One command settles it on pi3: `sudo grep -n 'entrypoints.web' /root/dmz-infrastructure/docker-compose.yml`.)

### Fix spec
- **File:** `docker-compose.yml`, labels block — add:
  ```
  - "traefik.http.routers.itworks-http.rule=Host(`itworks.dev`)"
  - "traefik.http.routers.itworks-http.entrypoints=web"
  - "traefik.http.routers.itworks-http.middlewares=https-redirect@file"
  ```
  `https-redirect@file` is a live pi3 middleware (`stacks/traefik-dynamic/pi3-traefik-dynamic.yml:35-38`: `https-redirect: redirectScheme: {scheme: https, permanent: true}`), so no new middleware is needed. wx-portal's local `redirectscheme` form is the fallback if the file provider is not loaded.
- **Check:** after deploy, `curl -sS -o /dev/null -w '%{http_code}\n' http://itworks.dev/` → 301. Today it would be 404.
- **Must not change:** the two existing `websecure` routers, their priorities (10 / 1), `authentik-auth@file` on the admin router, the `itworks-secheaders` middleware. The redirect router carries no service and must not get the secheaders or auth middleware.

---

## I4 — dmz-public peer can forge the admin header to port 8080
**Verdict: CONFIRMED as to exposure. Severity and fix belong to lane A — no fix spec written here.**

Two narrow facts verified:
1. `docker-compose.yml:13-14` joins `dmz-infrastructure_dmz-public` and `:18` sets `loadbalancer.server.port=8080`; no `ports:` key exists anywhere in the file. The container is therefore unreachable from the internet directly but **is** reachable at `itworks:8080` by every other container on that shared bridge. The runbook check at `README.md:36` ("no port is published; Traefik is the only way in") is true of the host, not of peers.
2. The cited note exists and says what the report says it says — `claude-knowledge-base/docs/superpowers/plans/2026-07-25-n8n-rotation-followups.md:367`:
   > "proven 2026-07-25 that a container on `dmz-infrastructure_dmz-public` can forge `X-authentik-groups: qr-portal-admins` straight to `prlx:3000` and get cross-tenant admin (Traefik bypassed). Triaged as container-foothold-required (NOT internet-reachable ...)"

   and `:413`: "this direct-to-container test bypasses Traefik, so it still succeeds — the middleware only protects traffic THROUGH Traefik. The real fix for the container-network vector is **network segmentation**." Option (b) there — a Traefik-injected shared-secret header — is the estate's smaller precedent.

**Pointer:** the application-side trust decision (`internal/server/server.go:125` admin guard, `ITWORKS_TRUST_PROXY: "1"` and `X-Real-Ip` for the rate limiter) is lane A's finding; defer severity and remedy to lane A.

---

## I5 — Image does not build for linux/amd64 on an arm64 host
**Verdict: CONFIRMED. Final severity: ADVISORY** (not IMPORTANT: the ship path is unaffected — `deploy.sh:23` builds on pi3, which is native amd64; nothing in the rubric's IMPORTANT list is hit).

Rerun on this Mac, Docker running:
```
docker build --platform linux/amd64 -t itworks-audit:verifyC .
...
#12 ERROR: process "/bin/sh -c go mod download" did not complete successfully: exit code: 2
  (Go runtime crash + stack trace, runtime/asm_amd64.s, under emulation)
Dockerfile:7  >>> RUN go mod download
```
Reproduced the report's failure exactly, at the same line.

`deploy.sh:23` confirms the real build happens on the target: `ssh "${REMOTE_HOST}" "cd ${REMOTE_DIR} && docker compose build --pull && docker compose up -d"`, `REMOTE_HOST="pi3"` (amd64), so tonight's deploy is not blocked.

Cross-compilation works and was proven, not asserted. Scratch Dockerfile (`scratchpad/audit/xbuild/Dockerfile`) = the repo's Dockerfile with two edits:
```
FROM --platform=$BUILDPLATFORM golang:1.26-alpine@sha256:ce864e... AS build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o /out/itworks ./cmd/itworks
```
Result: `docker build --platform linux/amd64 -f xbuild/Dockerfile <repo>` → **exit=0**, and `docker image inspect --format '{{.Os}}/{{.Architecture}}'` → **`linux/amd64`**. Pure Go (`CGO_ENABLED=0` already at `Dockerfile:9`, `modernc.org/sqlite` needs no cgo), so this is free.

### Fix spec
- **File:** `Dockerfile:4` add `--platform=$BUILDPLATFORM` to the build stage; `Dockerfile:9` add `GOOS=linux GOARCH=amd64`. Optionally `platform: linux/amd64` on the compose service so a stray local build is honest.
- **Check:** `docker build --platform linux/amd64 .` on the Mac. Fails today at `go mod download`; passes after (proven).
- **Must not change:** both base image digests (verified multi-arch, amd64 present), `CGO_ENABLED=0`, the distroless final stage, the `--chown=65532:65532 /data` step, `ENTRYPOINT ["/itworks"]`.

---

## A2 — No healthcheck, and the final image has nothing to run one
**Verdict: CONFIRMED. Final severity: ADVISORY.**

`docker-compose.yml` has no `healthcheck:` key; `Dockerfile:18` is `gcr.io/distroless/static-debian12:nonroot` — no shell, no curl, no wget, so the obvious `healthcheck: test: curl ...` would fail permanently. `/healthz` exists (`internal/server/server.go:96`) and `deploy.sh:26` curls it exactly once. `restart: unless-stopped` only reacts to the process exiting; a hung-but-alive process stays in Traefik's rotation.

Options, in order of cost:
1. **Traefik label (recommended):** `traefik.http.services.itworks.loadbalancer.healthcheck.path=/healthz` plus `...healthcheck.interval=10s`. One line, nothing added to the image, and it takes an unhealthy container out of rotation — which is the actual goal.
2. **Compose healthcheck with a self-probe:** add a `-healthcheck` flag to `cmd/itworks` and `HEALTHCHECK CMD ["/itworks", "-healthcheck"]`. Needs a code change, lets Compose restart the container.
3. **A tiny second Go binary** built in the same build stage and copied in. Works, but strictly more than option 1 buys.
Do not add a shell or curl to the final image.

---

## Items carried from the report without independent re-verification
A1 (IP in logs), A3 (rsync excludes), A4 (only pending rows capped), A5 (network declared by key) were read and are consistent with the files, but are ADVISORY and were not the assigned points to settle. A5's own text already concedes it works; leave it cosmetic. The report's "checks that passed" list — the `--go` guard, the `pi3` alias, non-root distroless, digest pins, `CREATE TABLE IF NOT EXISTS` restart safety — was spot-checked against `deploy.sh:6-12`, `Dockerfile`, and `docker-compose.yml` and holds.

---

## Summary

| Suspicion | Verdict | Final severity |
|---|---|---|
| C1 Backup: WAL `cp` unrestorable, no restore procedure | PARTIAL (hazard confirmed and worse; "never leaves pi3" REFUTED — kopia covers `/var/lib/docker/volumes` to B2 nightly) | IMPORTANT |
| I1 Graceful shutdown never completes | CONFIRMED | IMPORTANT |
| I2 Rollback not runnable (no tags, mutable image tag) | CONFIRMED | IMPORTANT |
| I3 No HTTP→HTTPS redirect router | CONFIRMED (no global pi3 entrypoint redirect found) | IMPORTANT |
| I4 dmz-public peer reaches `itworks:8080` and can forge admin header | CONFIRMED (exposure only) | defer to lane A |
| I5 `--platform linux/amd64` build fails on the Mac | CONFIRMED (ship path unaffected; cross-compile fix proven) | ADVISORY |
| A2 No healthcheck, distroless has no probe tool | CONFIRMED | ADVISORY |
