# itworks.dev site contract

A static site. A Go command reads the entry files, renders HTML and SVG into
a directory, and GitHub Pages serves that directory at https://itworks.dev.
There is no server, no database and no container. Go 1.26, standard library
only, no dependencies at all.

The site makes NO model calls, has NO accounts, sends NO email, and holds NO
credentials in any file. UI copy is plain English, US spelling, never the
phrase "vibe check", and carries no dashes of any kind ("-", "–", "—") inside
any text a visitor can read. Attributes, CSS, code and ISO dates such as
2026-09-07 are exempt: a date is data, not copy.

## Layout

```
cmd/build/                     the renderer: reads entries/, writes the site
internal/entry/                the record type, its rules, and the loader
internal/badge/                Color(auditDate, critical, now) string; Render(date, critical, color) []byte
web/templates/*.html           base.html (defines "head", "footer", "colhead", "ledgerrow"),
                               landing.html, wall.html, entry.html, notfound.html
web/static/site.css            the ONLY stylesheet; no inline styles or scripts anywhere
web/embed.go                   package web, //go:embed templates static
entries/*.json                 one file per entry, named by its id
fixtures/seed-msp-sentinel.json  a sample entry
docs/SPEC.md                   this file
```

## Commands

| Command | What it does |
|---|---|
| `go run ./cmd/build -out dist` | render the whole site into dist |
| `go run ./cmd/build -check` | validate entries/ and write nothing; exits non zero listing every problem |
| `go run ./cmd/build -today 2026-01-31` | render against a fixed reference date instead of today in UTC |
| `go run ./cmd/build -entries <dir>` | read the entry files from another directory |
| `go test ./...` | the tests |

## Output

| Path | What it is |
|---|---|
| index.html | landing page; the masthead badge is the newest approved entry, or the captioned example when the wall is empty |
| wall/index.html | every entry, newest approval first, ties broken by id ascending |
| e/`<id>`/index.html | one entry |
| badge/`<id>`.svg | the badge for one entry |
| badge/example-green.svg, badge/example-amber.svg, badge/example-red.svg | the three specimens the landing page shows |
| api/entries.json | `{"entries":[...]}`, same order as the wall |
| 404.html | GitHub Pages serves this for an unknown path |
| static/ | the stylesheet and the self hosted fonts, copied from web/static |
| CNAME | `itworks.dev` |
| .nojekyll | empty; it stops GitHub running Jekyll over the artifact |

## The entry file

One entry is one file: `entries/<id>.json`. The id is the file name stem and
must match `^[a-z2-7]{12}$`, twelve characters of the lowercase base32
alphabet. There is no `id` key inside the file; the file is decoded with
unknown fields rejected, so an `id` key is an error. Two files may not carry
the same id.

A merged pull request is an approval. The approval date is the commit date of
the commit that added the file, read with
`git log --diff-filter=A --format=%cI -1 -- <path>`. When git is missing or
the file was never committed, the build falls back to the file modification
time and prints a warning. Deleting the file hides the entry: its page and
its badge are gone on the next build. A pending entry has no badge; the badge
starts rendering when the pull request is merged.

### Rules

Every field is required. A field that is missing, of the wrong type, or not
listed here is an error, and so is a file holding more than one JSON value.

| Field | Type | Rule |
|---|---|---|
| name | string | trimmed, 1 to 60 characters, no control characters and no invisible format characters |
| summary | string | trimmed, 1 to 160 characters, same character rule as name |
| source | string | exactly "public" or "closed" |
| repo_url | string | may be ""; when set, at most 200 characters, parses as a URL, scheme exactly http or https, non empty host, no username or password, no fragment; any host and any port are allowed; MUST be "" when source is "closed" |
| audit_tier | string | exactly "checkpoint", "closeout" or "audit" |
| audit_date | string | "YYYY-MM-DD", a real calendar date, not after today in UTC, not before 2025-01-01 |
| found | int | 0 to 9999 |
| fixed | int | 0 to 9999 |
| accepted | int | 0 to 9999 |
| critical_open | int | 0 to 9999 |
| critical_accepted | int | 0 to 9999 |

Across fields: `fixed + accepted <= found`, `critical_open <= found`, and
`critical_open + critical_accepted <= found`.

The character rule rejects control characters (Unicode category Cc) and
format characters (category Cf), which covers the bidi overrides and isolates
U+202A to U+202E and U+2066 to U+2069, the zero width marks U+200B to U+200F
and the byte order mark U+FEFF. Those runes let a stored name read as a
different name on the page. Letters with accents, combining marks, other
scripts and emoji are all legal.

### Example

```json
{
  "name": "MSP Sentinel",
  "summary": "Monitors managed service infrastructure, records findings deterministically, and delivers alerts and reports to operators.",
  "source": "closed",
  "repo_url": "",
  "audit_tier": "audit",
  "audit_date": "2026-09-07",
  "found": 56,
  "fixed": 56,
  "accepted": 0,
  "critical_open": 0,
  "critical_accepted": 0
}
```

## api/entries.json

```json
{"entries":[{"id":"...","name":"...","summary":"...","source":"...","repo_url":"...",
 "audit_tier":"...","audit_date":"YYYY-MM-DD","found":0,"fixed":0,"accepted":0,
 "critical_open":0,"critical_accepted":0,"age_days":0,"badge_color":"green",
 "badge_url":"https://itworks.dev/badge/<id>.svg","entry_url":"https://itworks.dev/e/<id>/"}]}
```

`badge_url` and `entry_url` are absolute because they are quoted away from
the site. Every link inside a page is root relative, so a local preview
works.

## Badge rules (internal/badge)

The critical count the badge reads is `critical_open + critical_accepted`. A
critical the owner accepted is not a critical that was fixed, and the badge
is the honest signal, so an accepted critical keeps the plate red.

`Color`: if the critical count is above zero, "red"; else if the UTC date of
the reference day minus audit_date is 30 days or more, "amber"; else "green".
Boundary: 29 days is green, 30 days is amber. Red wins over amber.

Colors (all pass WCAG AA at 11 px): green plate `#1a7f37` with white text,
amber plate `#d29922` with ink text `#1f1f1f`, red plate `#b3261e` with white
text, label background `#24292f` with white text. The old `#3fb950` and
`#f85149` plates failed AA with white text and must not return.

`Render` returns the Ledger badge (design source:
docs/design/ledger-mock-v2.html, the three `<symbol>` sprites): height 20,
content sized width; left plate `#1f1f1f` with the label `itworks.dev` in
white; a 1 px joint hairline; then the state plate carrying a state glyph
(green: filled disc; amber: diamond; red: triangle) 10 px in, and the text
12 px after the glyph, 12 px trailing. Right text is
`<YYYY-MM-DD> · <N> critical` for green and red and
`<YYYY-MM-DD> · <N> critical · stale` for amber (the middle dot is U+00B7,
not a dash), so the amber state survives grayscale and a screen reader.
Font: `-apple-system,BlinkMacSystemFont,Segoe UI,Helvetica,Arial,sans-serif`
600 weight, size 11, with `textLength` pinned to the computed width so layout
is stable across platforms. `<title>` reads
`Audit <date>, <N> critical open, <color>` (amber:
`Audit <date>, stale, <N> critical open, amber`). No external references in
the SVG.

The entry page prints `critical open` and `critical accepted by the owner` as
two separate lines. The two numbers are never added together in visible copy;
only the badge colour and the badge count use the sum.

## Template contract (html/template)

Files are parsed together with `template.ParseFS(web.FS, "templates/*.html")`
and executed by file name. base.html defines `{{define "head"}}` (doctype
through `<body>` open, takes a title string), `{{define "footer"}}`,
`{{define "colhead"}}` and `{{define "ledgerrow"}}`. Nothing is ever passed
as `template.HTML`: every badge on a page is an `<img>` pointing at a file
under /badge/, so contextual escaping is never skipped. Every repo link a
submitter supplied carries `rel="nofollow noopener noreferrer"`.

```go
type view struct {
    ID, Name, Summary, Source, RepoURL, AuditTier, AuditDate string
    Found, Fixed, Accepted, CriticalOpen, CriticalAccepted, AgeDays int
    BadgeColor string   // green|amber|red
    BadgeURL, EntryURL  string  // absolute, for the feed and the README snippet
    BadgePath, EntryPath string // root relative, for markup
}
type landingData struct { Title string; InstallCommands []string; Entries []view /* newest 3, for the wall preview */ }
type wallData    struct { Title string; Entries []view }
type entryData   struct { Title string; Entry view }
type pageData    struct { Title string }
```

InstallCommands is exactly:
`claude plugin marketplace add parallaxintelligencepartnership/itworks` and
`claude plugin install itworks@itworks`.

## Design decisions this file records

- The badge colour rules above, including the 30 day boundary and red
  winning over amber.
- The darkened plates: three design judges measured the original plates at
  2.5 to 3.4 to 1 against white text, failing AA on the one artifact that
  leaves the site, so the plates were darkened and the amber badge carries
  the word stale. Amber must not rely on hue alone.
- ISO dates are exempt from the no dash rule in visitor readable text,
  because the audit date is data the badge and the wall must show verbatim.
- Entry ids are 12 characters of lowercase base32. They were unguessable
  strings when pending entries were unlisted; they stay the id format
  because they are already in READMEs.
- The Ledger design direction, with the notes recorded in
  docs/DESIGN-NOTES.md.
