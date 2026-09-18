# Map

## Run
`go run ./cmd/build -out dist` then `python3 -m http.server -d dist 8080` and open http://localhost:8080
Check the entries without writing anything: `go run ./cmd/build -check`
Render against a fixed date: `go run ./cmd/build -out dist -today 2026-01-31`; read entries from elsewhere: `-entries <dir>`

## Test
`go test ./...` (also `go vet ./...` and `gofmt -l .` before a checkpoint)
`node --test web/static/embed_test.mjs`
One package: `go test ./internal/badge -run TestName`

## Layout
| Path | What lives there |
|---|---|
| cmd/build/main.go | the renderer: flags, load, sort, write every output file, plus sitemap.xml and robots.txt |
| cmd/build/view.go | the shape the templates and api/entries.json see, the per page head (canonical, description) and the JSON-LD |
| internal/entry/entry.go | the entry file contract: fields, limits, allowed values, error wording |
| internal/entry/load.go | reads entries/*.json, takes the id from the file name, takes the approval date from the commit that added the file |
| internal/badge/badge.go | SVG rendering and the green/amber/red rules |
| web/templates/ | the public notice board pages; base.html holds the head, the footer, the brand mark, the split flap, the column head and the register row, and every page template calls them |
| web/static/ | one stylesheet (the notice system, no JavaScript on any page), self hosted fonts, embed.js for other sites; embedded via web/embed.go |
| entries/ | the records: one JSON file per entry, added by pull request |
| fixtures/ | a sample entry, used by the build tests |
| .github/workflows/ | pages.yml builds and deploys; check.yml gates pull requests |
| docs/SPEC.md, docs/DESIGN-NOTES.md | the written contract, and the design decisions behind the public notice board (round 3) |
| docs/design/round3/notice/ | the mocks the build follows, and impl/ with the built pages at 1440 and 390 |

## Environment
- GitHub Pages, repo under the parallaxintelligencepartnership org, Pages source set to GitHub Actions.
- Custom domain itworks.build; the build writes CNAME into the artifact, and Enforce HTTPS goes on once the certificate issues.
- The site rebuilds on every push to main, daily at 05:17 UTC (cron 17 5 * * *), and on demand; the daily build is what turns a badge amber on day 30.
- There is no host, no container and no database; the whole site is files in the artifact.
- Domain itworks.build bought 2026-09-17 (Spaceship, standard price). The GitHub remote is created at the first ship; Gitea (origin) stays the mirror.

## Gotchas
- 2026-09-11 | "pi" in pi1/pi2/pi3 means Parallax Intelligence, not Raspberry Pi; base images must be amd64 | run `uname -m` on the host before pinning any image
- 2026-09-11 | admin POSTs need both X-Authentik-Username and a same-origin Origin header, so a plain browser tab gets 403 locally | test admin with curl carrying both headers, or through Traefik once deployed
- 2026-09-12 | the default ITWORKS_DB path /data/itworks.db does not exist on a Mac, so a bare `go run` fails at startup | always set ITWORKS_DB for local runs (data/ is gitignored)
- 2026-09-11 | the rate limiter lives in process memory; a restart resets every counter | accepted in PROJECT.md; do not read a post-restart 201 as a limiter bug
- 2026-09-16 | the site is static now; there is no server, no SQLite and no admin route, and pending entries have no badge until their PR is merged | do not look for the old routes or the pi3 deploy
