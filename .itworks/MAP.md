# Map

## Run
`ITWORKS_DB=./data/itworks.db ITWORKS_BASE_URL=http://localhost:8080 go run ./cmd/itworks` then open http://localhost:8080
Env var names: ITWORKS_ADDR, ITWORKS_DB, ITWORKS_BASE_URL, ITWORKS_TRUST_PROXY, ITWORKS_ADMIN_USERS
Seed one entry: `scripts/seed.sh` (POSTs fixtures/seed-msp-sentinel.json); approve it with `curl -X POST -H "X-Authentik-Username: matthew" -H "Origin: http://localhost:8080" localhost:8080/admin/entries/<id>/approve`

## Test
`go test ./...` (also `go vet ./...` and `govulncheck ./...` before a checkpoint)
One package: `go test ./internal/badge -run TestName`

## Layout
| Path | What lives there |
|---|---|
| cmd/itworks/main.go | entry point; reads the env vars, opens the store, starts the server |
| internal/server/server.go | route table, rate limiter wiring, admin guard (header plus same-origin check) |
| internal/server/handlers.go | every HTTP handler: wall, entry page, API create and list, badge, admin |
| internal/server/validate.go | the submit payload contract; unknown fields and oversize bodies are rejected here |
| internal/server/ratelimit.go | per-IP limiter, in memory, per process |
| internal/store/store.go | all SQL; the only package that opens the SQLite file |
| internal/badge/badge.go | SVG rendering and the green/amber/red rules |
| web/templates/, web/static/ | html/template pages, one stylesheet, self-hosted fonts; embedded via web/embed.go |
| fixtures/, scripts/seed.sh | sample entry and the script that posts it |
| docs/SPEC.md, docs/DESIGN-NOTES.md | the written spec and the design decisions behind the Ledger look |
| Dockerfile, docker-compose.yml, deploy.sh | build, Traefik labels, and the rsync-then-compose deploy (refuses to run without --go) |

## Environment
- pi3 is an x86_64 Ubuntu 24.04 DMZ host running Traefik; it is not a Raspberry Pi.
- Deploy target is pi3:/opt/itworks via `./deploy.sh --go`; no host port is published, Traefik routes https://itworks.dev to the container.
- /admin sits behind Authentik forward-auth at Traefik (authentik-auth@file); the app also checks ITWORKS_ADMIN_USERS against the forwarded username.
- Data is one SQLite file on the Docker volume itworks-data, mounted at /data.
- As of 2026-09-12 the domain is not bought and nothing is deployed; the site has only run locally.

## Gotchas
- 2026-09-11 | "pi" in pi1/pi2/pi3 means Parallax Intelligence, not Raspberry Pi; base images must be amd64 | run `uname -m` on the host before pinning any image
- 2026-09-11 | admin POSTs need both X-Authentik-Username and a same-origin Origin header, so a plain browser tab gets 403 locally | test admin with curl carrying both headers, or through Traefik once deployed
- 2026-09-12 | the default ITWORKS_DB path /data/itworks.db does not exist on a Mac, so a bare `go run` fails at startup | always set ITWORKS_DB for local runs (data/ is gitignored)
- 2026-09-11 | the rate limiter lives in process memory; a restart resets every counter | accepted in PROJECT.md; do not read a post-restart 201 as a limiter bug
