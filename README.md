# itworks.dev

A public wall of AI coding agent audit results. Submitters POST an audit summary and get a badge to embed in their repo; entries need admin approval before they show on the wall.

## Local run

```
go run ./cmd/itworks
```

Environment variables (all optional, defaults shown):

```
ITWORKS_ADDR=:8080
ITWORKS_DB=/data/itworks.db
ITWORKS_BASE_URL=https://itworks.dev
ITWORKS_TRUST_PROXY=0
ITWORKS_ADMIN_USERS=matthew
```

For a local run, point `ITWORKS_DB` at a writable path in the repo, e.g. `ITWORKS_DB=./data/itworks.db go run ./cmd/itworks` (the `data/` directory is gitignored).

## Tests

```
go test ./...
```

## Runbook (first deploy)

1. On pi3, create the deploy directory: `ssh pi3 "mkdir -p /opt/itworks"`.
2. Point DNS: create an A record for `itworks.dev` to `185.187.235.55`.
3. From your machine, run the deploy: `./deploy.sh --go`. This rsyncs the repo to `pi3:/opt/itworks`, then builds and starts the stack there, then checks `https://itworks.dev/healthz`.
4. Submit the first entry (see `fixtures/seed-msp-sentinel.json` and `scripts/seed.sh`), then approve it at `https://itworks.dev/admin`. That page sits behind Authentik forward-auth on pi3 (`authentik-auth@file`), so you will be prompted to sign in there before you see the admin list. The app's own allowlist, `ITWORKS_ADMIN_USERS` in `docker-compose.yml`, must also match the `X-Authentik-Username` value Traefik forwards for you; verify this at first deploy, since a mismatch means Authentik lets you in but the app still returns 403.
5. Post-deploy checks:
   - `curl -m 5 http://185.187.235.55:8080/` must fail to connect (no port is published; Traefik is the only way in).
   - `curl https://itworks.dev/badge/<id>.svg` returns an SVG badge for the entry you just approved.

## Backup

The whole database is one SQLite file inside the `itworks-data` named volume. To copy it out:

```
docker run --rm -v itworks-data:/data -v "$(pwd)":/backup alpine \
  cp /data/itworks.db /backup/itworks-$(date +%Y%m%d).db
```

Run that on pi3, from `/opt/itworks`.

## Rollback

```
git checkout <previous-tag>
./deploy.sh --go
```

This re-syncs the older code to pi3 and rebuilds and restarts the container. The database in `itworks-data` is untouched by a rollback since it lives in a separate named volume, not in the repo checkout.

## Post deploy checks from checkpoint 1

Two findings can only be verified against the live Traefik route. Run both right after the first deploy and record the result in .itworks/REVIEWS.md.

1. Admin allowlist matches Authentik. Sign in as Matt and open https://itworks.dev/admin (expect 200). The value in ITWORKS_ADMIN_USERS must equal the X-authentik-username Traefik forwards; if the page is 403 for Matt, check the container log for `admin denied user=` and set the variable to that name in docker-compose.yml, then redeploy.
2. Rate limiting keys on the real client. Traefik must overwrite X-Real-Ip. Proof:

```
for i in $(seq 1 6); do curl -s -o /dev/null -w "%{http_code}\n" -H "X-Real-Ip: 10.0.0.$i" -H "Content-Type: application/json" -d @fixtures/seed-msp-sentinel.json https://itworks.dev/api/entries; done
```

The sixth line must be 429. If every line is 201, the header is trusted from the client and ITWORKS_TRUST_PROXY must key on the last X-Forwarded-For hop instead. The six test entries stay pending; hide them from /admin afterwards.
