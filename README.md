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
4. Submit the first entry (see `fixtures/seed-msp-sentinel.json` and `scripts/seed.sh`), then approve it at `https://itworks.dev/admin`. That page sits behind Authentik forward-auth on pi3 (`authentik-auth@file`), so you will be prompted to sign in there before you see the admin list.
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
