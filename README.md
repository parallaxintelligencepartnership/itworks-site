# itworks.build

A public wall of audit results for apps built with the itworks plugin. Each
entry carries the audit date, what was found, what was fixed, and how many
critical findings are still open, plus one SVG badge the owner can put in a
README.

The site is static. A Go command reads the entry files, writes HTML and SVG
into a directory, and GitHub Pages serves it. No server, no database, no
container, and no dependencies outside the Go standard library. The site
never runs a model.

## How an entry gets on the wall

1. Run the itworks closeout. It prints the JSON summary.
2. Open a pull request that adds exactly one file, `entries/<id>.json`, where
   `<id>` is 12 characters of lowercase base32 (a to z and 2 to 7). The
   closeout prints an id you can use.
3. Every pull request runs the tests, validates every entry, and checks that
   a submission adds one file under `entries/` and touches nothing else.
4. Merging the pull request is the approval. The entry appears on the next
   build, and the approval date is the date of the merge commit that added
   the file.
5. Deleting the file hides the entry. Its page and its badge are gone on the
   next build.

Until a pull request is merged there is no badge. The badge starts rendering
after the merge.

## The JSON contract

Every field, every limit, and the badge colour rules live in one place so
they cannot drift: [docs/SPEC.md](docs/SPEC.md). The rules table there is
checked against the code by a test, so if the two ever disagree the build
fails.

The short version: `name`, `summary`, `source`, `repo_url`, `audit_tier`,
`audit_date`, `found`, `fixed`, `accepted`, `critical_open` and
`critical_accepted`, all required, no other keys allowed, and no `id` key
because the file name is the id. See
[fixtures/seed-msp-sentinel.json](fixtures/seed-msp-sentinel.json) for a
complete example.

## Local commands

Build the site and read it in a browser:

```
go run ./cmd/build -out dist
python3 -m http.server -d dist 8080
```

Then open http://localhost:8080.

Validate the entries without writing anything, which is what a pull request
runs:

```
go run ./cmd/build -check
```

Run the tests:

```
go test ./...
```

Two more flags help when you are working on the badge: `-today 2026-01-31`
renders against a fixed date, so you can see the amber plate without waiting
30 days, and `-entries <dir>` reads the entry files from somewhere else.

## Embedding the wall on another site

GitHub Pages serves every file with `access-control-allow-origin: *`, so
`api/entries.json` is a plain, open-CORS JSON document at
`https://itworks.build/api/entries.json`
(see [docs/SPEC.md](docs/SPEC.md#apientriesjson) for its shape: `id`, `name`,
`summary`, `source`, `repo_url`, `audit_tier`, `audit_date`, `found`,
`fixed`, `accepted`, `critical_open`, `critical_accepted`, `age_days`,
`badge_color`, `badge_url`, `entry_url`, plus the feed-level `built_at`). A
build-time fetch of that URL works from any other Parallax site.

For a plain drop-in, add these two lines to a page:

```html
<div data-itworks-wall data-limit="5"></div>
<script src="https://itworks.build/static/embed.js" defer></script>
```

`static/embed.js` fetches the feed, takes the newest `data-limit` entries
(default 5), and renders each as a name, summary, audit tier and date, and
badge, all linking back to itworks.build. It has no dependencies and
inherits the host page's fonts and colours.

## Pointing itworks.build at GitHub Pages

DNS records at the registrar for `itworks.build`:

| Type | Name | Value |
|---|---|---|
| A | @ | 185.199.108.153 |
| A | @ | 185.199.109.153 |
| A | @ | 185.199.110.153 |
| A | @ | 185.199.111.153 |
| AAAA | @ | 2606:50c0:8000::153 |
| AAAA | @ | 2606:50c0:8001::153 |
| AAAA | @ | 2606:50c0:8002::153 |
| AAAA | @ | 2606:50c0:8003::153 |
| CNAME | www | parallaxintelligencepartnership.github.io |

Repository settings to flip, under Settings then Pages:

1. Source: GitHub Actions.
2. Custom domain: `itworks.build`. The build writes a `CNAME` file into the
   artifact, so the setting and the file agree.
3. Enforce HTTPS: turn it on once GitHub reports the certificate as issued.
   That takes a few minutes after the DNS records resolve.

The site rebuilds on every push to `main`, every day at 05:17 UTC, and on
demand from the Actions tab. The daily build is what turns a badge amber on
the thirtieth day after its audit.

## Rolling back

1. Find the last good commit on `main`: `git log --oneline main`.
2. `git revert <bad-sha>` (or `git revert -m 1 <merge-sha>` for a squash
   merge that is a single commit, plain revert works).
3. Push `main`.
4. The pages workflow redeploys automatically, or dispatch it from the
   Actions tab.
5. Confirm with `curl -sI https://itworks.build/ | head -1` and the site's
   `api/entries.json` `built_at`.

Ship tags are named `ship-YYYY-MM-DD` and mark the last known-good state.
