# itworks.dev service contract

Single Go binary (Go 1.26, stdlib + modernc.org/sqlite v1.58.0 pinned exact). One SQLite file. Static assets embedded. NO model calls, NO accounts, NO email, NO credentials in any file. UI copy: plain English, US spelling, never the phrase "vibe check", and no dashes of any kind ("-", "–", "—") inside any text a visitor can read (attributes, CSS, code, and ISO dates such as 2026-09-07 are exempt; dates are data, not copy).

## Layout
```
cmd/itworks/main.go            entrypoint: env config, open db, migrate, serve
internal/store/                sqlite open, migrate, CRUD (package store)
internal/badge/                Color(auditDate time.Time, criticalOpen int, now time.Time) string; Render(date string, criticalOpen int, color string) []byte
internal/server/               http handlers, validation, rate limiter, templates, embed
web/templates/*.html           base.html (defines "head" and "footer"), landing.html, wall.html, entry.html, admin.html
web/static/site.css            the ONLY stylesheet; no inline styles or scripts anywhere (CSP forbids them)
fixtures/seed-msp-sentinel.json  the first entry
scripts/seed.sh                POSTs a fixture to $ITWORKS_URL (default http://localhost:8080)
docs/SPEC.md                   this file
```
Embed: `//go:embed` of `web/templates/*.html` and `web/static/*` lives in package server (place a `web.go` in internal/server with `//go:embed ../../web` is NOT allowed by Go; instead put `embed.go` at repo root package `web` with `//go:embed templates static` under the `web/` directory, i.e. `web/embed.go` `package web` `var FS embed.FS`).

## Environment
| Var | Default | Meaning |
|---|---|---|
| ITWORKS_ADDR | :8080 | listen address |
| ITWORKS_DB | /data/itworks.db | sqlite path (WAL, busy_timeout 5000, MaxOpenConns 1) |
| ITWORKS_BASE_URL | https://itworks.dev | used to build badge_url and entry_url |
| ITWORKS_TRUST_PROXY | 0 | when 1, client IP = X-Real-Ip header (Traefik sets it); else RemoteAddr host |

## Data model (table entries)
id TEXT PK (12 chars, lowercase base32 alphabet `abcdefghijklmnopqrstuvwxyz234567`, crypto/rand), name TEXT, summary TEXT, source TEXT, repo_url TEXT, audit_tier TEXT, audit_date TEXT (YYYY-MM-DD), found INT, fixed INT, accepted INT, critical_open INT, status TEXT (pending|approved|hidden), created_at TEXT (RFC3339 UTC), approved_at TEXT NULL. No IP addresses are stored, ever.

## Routes
| Route | Behavior |
|---|---|
| GET / | landing page (template landing) |
| GET /wall | wall page, approved entries, newest approved_at first |
| GET /e/{id} | entry page; 404 unless status approved |
| GET /api/entries | JSON `{"entries":[EntryView...]}`, approved only, same order |
| POST /api/entries | submit; see validation; 201 JSON `{"id","badge_url","entry_url","status":"pending"}` |
| GET /badge/{id}.svg | SVG badge; 200 for pending or approved, 404 for hidden or unknown; headers `Content-Type: image/svg+xml; charset=utf-8`, `Cache-Control: public, max-age=3600` |
| GET /static/site.css | embedded css, `Cache-Control: public, max-age=86400` |
| GET /healthz | 200 `ok` |
| GET /admin | HTML list: pending first, then approved, then hidden; each row has two forms |
| POST /admin/entries/{id}/approve | status=approved, approved_at=now; 303 to /admin |
| POST /admin/entries/{id}/hide | status=hidden; 303 to /admin |
Everything else 404. Method mismatch 405.

Admin guard (all /admin routes): request must carry header `X-Authentik-Username` with a non empty value, else 403. Admin POSTs must also carry an `Origin` header whose host equals the request Host, else 403. Traefik forward-auth is the primary guard; this is defense in depth.

Security headers on every response: `Content-Security-Policy: default-src 'none'; style-src 'self'; img-src 'self' data:; form-action 'self'; base-uri 'none'; frame-ancestors 'none'`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: strict-origin-when-cross-origin`, `X-Frame-Options: DENY`. The badge route may relax CSP to `default-src 'none'` only.

## POST /api/entries validation
Content-Type must start with `application/json` else 415. Body read through `http.MaxBytesReader` at 8192 bytes; over cap → 413. Decoder uses `DisallowUnknownFields`; any unknown field, syntax error, or missing required field → 400 with JSON `{"error":"<plain English>"}`. Required fields:
| Field | Rule |
|---|---|
| name | string, trimmed, 1..60 chars, printable only (no control chars) |
| summary | string, trimmed, 1..160 chars, printable only |
| source | exactly "public" or "closed" |
| repo_url | string, optional (may be omitted or ""); if present: parses with net/url, scheme https, non empty host, ≤ 200 chars; MUST be empty when source is "closed" |
| audit_tier | exactly "checkpoint", "closeout", or "audit" |
| audit_date | "YYYY-MM-DD", valid calendar date, not after today (UTC), not before 2025-01-01 |
| found, fixed, accepted, critical_open | integers 0..9999; fixed + accepted ≤ found; critical_open ≤ found |
Abuse limits: per IP limiter, 5 POSTs per rolling hour (in memory, token bucket or sliding window, pruned), 6th → 429 with `Retry-After` seconds. Global cap: if pending count ≥ 500 → 503 `{"error":"submissions are paused, try again later"}`. Log one line per POST (ip, status) and per admin action; never log bodies.

## Badge rules (internal/badge)
`Color`: if criticalOpen > 0 → "red"; else if the UTC date of now minus audit_date ≥ 30 days → "amber"; else "green". Boundary: 29 days → green, 30 days → amber. Red wins over amber.
Colors: green `#3fb950`, amber `#d29922`, red `#f85149`, label background `#24292f`, text `#ffffff`.
`Render` returns a flat shields style SVG, height 20, left label text `itworks.dev audit`, right text `<YYYY-MM-DD> · <N> critical open` (the middle dot is U+00B7, not a dash). Width from text length at 6.5 px per char plus 10 px padding per side, font family `Verdana,Geneva,DejaVu Sans,sans-serif` size 11, `<title>` reads `Audit <date>, <N> critical open, <color>`. Right segment fill is the color. No external references in the SVG.

## Template contract (html/template)
Files parsed together with `template.ParseFS(web.FS, "templates/*.html")`; pages executed by file name ("landing.html" etc.). base.html defines `{{define "head"}}` (doctype through `<body>` open, takes `.Title` string) and `{{define "footer"}}` (site footer through `</html>`). Data passed to each page:
```go
type EntryView struct {
    ID, Name, Summary, Source, RepoURL, AuditTier, AuditDate string
    Found, Fixed, Accepted, CriticalOpen, AgeDays int
    BadgeColor string   // green|amber|red
    BadgeURL, EntryURL string
}
type LandingData struct { Title string; EntryCount int; Badges struct{ Green, Amber, Red template.HTML }; InstallCommands []string }
type WallData    struct { Title string; Entries []EntryView }
type EntryData   struct { Title string; Entry EntryView }
type AdminData   struct { Title string; Pending, Approved, Hidden []EntryView }
```
InstallCommands is exactly: `claude plugin marketplace add parallaxintelligencepartnership/vibecheck` and `claude plugin install vibecheck@vibecheck`.

## Publish payload (what the plugin sends, nothing else)
```json
{"name":"MSP Sentinel","summary":"...","source":"closed","repo_url":"","audit_tier":"audit","audit_date":"2026-09-07","found":56,"fixed":56,"accepted":0,"critical_open":0}
```
