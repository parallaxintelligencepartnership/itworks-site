# Lane B report: Records and rendering (lens real-data)

Repo: /Users/matthew/parallax-private/Projects/itworks-site @ 5a87acd. Read only. No repo file edited.

Headline: HTML escaping and SVG construction are sound, and validation rejects almost everything it should. The soft spots are all in the same place: **what the record lets a viewer believe**. Unicode direction and invisible characters pass validation and reach the wall raw; a repo URL with userinfo passes and renders as a link that reads like GitHub and goes somewhere else; and hiding an entry does not retract the badge already cached in READMEs.

---

## 1. Suspicions

### B1. Bidi and zero-width characters pass validation and render raw on the wall
- **Where:** internal/server/validate.go:181-190 (`isPrintable` only rejects `unicode.IsControl`), rendered at web/templates/base.html:38 and :41, entry.html:7-8, admin.html:3.
- **Input (verified, 201 Created):**
  ```json
  {"name":"<script>alert(1)</script> & \"x\" ]]> ‮evil","summary":"...","source":"public","repo_url":"https://example.com/a","audit_tier":"audit","audit_date":"2026-09-16","found":5,"fixed":1,"accepted":1,"critical_open":0}
  ```
- **Wrong outcome:** `U+202E RIGHT TO LEFT OVERRIDE` is stored and emitted verbatim into the wall row (observed: `&lt;script&gt;...&#34;x&#34; ]]&gt; ‮evil`). Everything after it renders reversed, so the displayed project name is not the stored name. The same class of character (U+200B/U+200D zero width, U+2066-U+2069 isolates, U+00A0) also survives `strings.TrimSpace` and the length check, so a name can be padded with invisible characters or made to display as a different, well known project. The admin screen shows the same reversed string, so the approver approves what they see, not what is stored.
- **Severity: IMPORTANT.** A viewer believes a displayed name that the record does not contain. Not CRITICAL only because nothing executes and the counts themselves are untouched.
- **Repro:** start a local instance, POST the payload above, approve the id, `curl localhost:PORT/wall | grep 'class="name"'`.

### B2. repo_url with userinfo is accepted and rendered as the "repo" link
- **Where:** internal/server/validate.go:98-113 (checks only `u.Scheme == "https"` and `u.Host != ""`; `u.User` is never inspected), rendered at web/templates/base.html:39, entry.html:17, admin.html:4.
- **Input (verified, 201 Created):** `"source":"public","repo_url":"https://github.com@evil.example.com/x"`
- **Wrong outcome:** the wall renders `<a href="https://github.com@evil.example.com/x">repo</a>`. The credible looking prefix is the userinfo; the request goes to `evil.example.com`. On the wall the link text is just the word "repo" and the full URL is never shown, so a visitor doing the one check the wall invites ("is the source really published?") is sent to an attacker host with a URL that reads as GitHub in the status bar. This is the site's own trust affordance being used against the visitor. The URL also has no length-vs-rune distinction issue but is never trimmed and never scanned for invisible characters (a zero width inside the host is percent escaped by Go on output, which merely breaks the link).
- **Severity: IMPORTANT** (CRITICAL if you count the repo link as part of the published trust signal, which is arguable; it does not execute for other visitors, so IMPORTANT per the rubric).
- **Repro:** POST the payload, approve, `curl .../wall | grep 'class="src"'`.

### B3. No `rel` on any visitor-supplied link, and no `nofollow`
- **Where:** web/templates/base.html:39, entry.html:17, admin.html:4.
- **Wrong outcome:** every approved entry gets a plain dofollow outbound link from itworks.dev. `nofollow ugc` is the normal defense for a link a stranger placed on your page; without it the wall is a link farm target and approval becomes an SEO favor, which changes who wants to be on the wall. `noopener noreferrer` matters less here because there is no `target="_blank"`, but adding it costs nothing.
- **Severity: IMPORTANT** (reputational and incentive damage to the product, not a stored-data error).
- **Repro:** `grep -n 'rel=' web/templates/*.html` returns only the stylesheet link.

### B4. Hiding an entry does not retract its badge for an hour, and cannot retract it from GitHub's camo at all
- **Where:** internal/server/handlers.go:169 (`Cache-Control: public, max-age=3600` on the badge) plus store.Hide (internal/store/store.go:238).
- **Wrong outcome:** the badge is the product. When Matt hides a fraudulent entry, `/badge/{id}.svg` starts returning 404, but every shared cache and every copy already fetched keeps serving the last green SVG for up to an hour. A README badge is served through GitHub's camo proxy, which fetches the SVG over TLS, strips cookies and prevents the origin from seeing the reader, and **re-serves the image from its own cache on its own schedule**; it honors upstream cache headers loosely and offers no purge to a third party origin. Camo also does not sanitize SVG, so the origin is the only sanitizer, and it does not re-check within the hour. So a retraction is not reliably visible to the audience the badge exists for, and nobody is told.
- **Severity: IMPORTANT.** The rubric would make a silently wrong public trust signal CRITICAL; it lands at IMPORTANT because it is a bounded cache window on a deliberately cacheable asset, not a stored record that is wrong. Worth a decision (shorter max-age for pending, or `no-store` until approved).
- **Repro:** fetch a badge, hide the entry, fetch again through any shared cache inside the hour.

### B5. `Render` builds the SVG with `fmt.Sprintf` and escapes nothing
- **Where:** internal/badge/badge.go:133-155.
- **Current state: not exploitable.** The only interpolated values are `date` (validated `YYYY-MM-DD` and round tripped through `time.Parse`, validate.go:127-130) and integers and a closed color set. Verified: the badge for the injection entry above contains no attacker text, and the response carries `Content-Type: image/svg+xml; charset=utf-8` plus `Content-Security-Policy: default-src 'none'` (handlers.go:167-168), which blocks script, foreignObject fetches and xlink:href in a directly navigated SVG.
- **Wrong outcome if it changes:** the day anyone puts `Name` in the badge (an obvious next feature, and the alt text already pairs name with badge) a payload of `]]></text><script>` or `"` lands in an XML document served from the site's own origin. An SVG opened directly at `/badge/x.svg` is a same origin document; only the badge CSP stands between it and script execution. There is no test asserting the SVG is well formed XML for hostile input, and no escaping helper to reach for.
- **Severity: IMPORTANT** (latent; a defensive note, not a live bug). Recommend building the SVG with a `text/template` or an explicit `xmlEscape` on every interpolated string now, while it is free.

### B6. Approve has no source-state precondition, so `hide` then `approve` silently republishes
- **Where:** internal/store/store.go:227-234 (`UPDATE entries SET status=?, approved_at=? WHERE id=?`, no `WHERE status=...`).
- **Verified:** approve, approve again, hide, approve all return 303 and the entry is back on the wall; approving an unknown id returns 404 (`checkRowsAffected`). So the operations are idempotent and non-destructive, which is what the invariant asked.
- **Wrong outcome:** re-approving rewrites `approved_at` to now, and `ListApproved` orders by `approved_at DESC` (store.go:205), so a re-approval jumps an old entry to the top of the wall. A hidden entry is one misclick from being live again with no confirmation and no audit trail beyond a log line (handlers.go:277-283). There is no `hidden_at`, no `hidden_reason`, and no record that an entry was ever hidden.
- **Severity: IMPORTANT** (moderation history is unrecoverable; the wall order can be changed without new information).

### B7. Wall order ties are unstable
- **Where:** internal/store/store.go:205, `ORDER BY approved_at DESC` where `approved_at` is `time.RFC3339` at **second** resolution (store.go:228).
- **Wrong outcome:** two entries approved in the same second (normal when Matt clears a queue) have no defined relative order and can swap between page loads, because SQLite gives no tie break. Add `, created_at DESC, id DESC`, or store RFC3339Nano.
- **Severity: MINOR.**

### B8. Pending cap is a check-then-act across two statements
- **Where:** internal/server/handlers.go:118-132 (`CountPending` then `Create`).
- **Wrong outcome:** concurrent POSTs can each read 499 and both insert, overshooting the cap. With `SetMaxOpenConns(1)` (store.go:75) the statements serialize, so overshoot is bounded by in-flight requests, and nothing is dropped or overwritten: the invariant "never drops or overwrites an existing row" holds. Not material at this scale.
- **Severity: MINOR** (correct as a design note: an `INSERT ... WHERE (SELECT COUNT(*) ...) < cap` would close it).

### B9. `pendingCap` path returns 503 but a DB error on the count path returns 500 through a different writer
- **Where:** internal/server/handlers.go:118-121 and 128-131 use `s.serverError` while every other exit uses `s.finishPost`. The 500 responses are therefore **not logged as a POST line** with the ip and status, which SPEC.md:62 requires ("Log one line per POST (ip, status)"). Small observability hole exactly on the path you would need the log for.
- **Severity: MINOR.**

### B10. Schema, WAL and pool: clean
- internal/store/store.go:90-112 is `CREATE TABLE IF NOT EXISTS` plus `CREATE INDEX IF NOT EXISTS`, no `DROP`, no `ALTER`, no destructive migration. Verified by restarting three times against the same `laneB.db` with rows present: rows survived, no error.
- DSN sets `busy_timeout(5000)` and `journal_mode(WAL)` (store.go:70) and `SetMaxOpenConns(1)` (store.go:75). WAL confirmed live (`-wal` and `-shm` files present). Note `SetMaxIdleConns` and `SetConnMaxLifetime` are left at defaults; with MaxOpen 1 that is fine, but it also means every page render serializes behind every write. Acceptable at this scale, worth knowing.
- **No finding.**

### B11. Id generation: correct, including the modulo
- internal/store/store.go:114-124: 12 bytes of `crypto/rand`, `idAlphabet[int(v)%32]`. 256 is an exact multiple of 32, so the modulo introduces **no** bias (the usual `%` bug does not apply here). Alphabet is exactly the lowercase base32 alphabet claimed by SPEC.md:29, length 12. Collisions retry up to 5 times against the PRIMARY KEY and then error out; no path overwrites another entry (`INSERT`, never `INSERT OR REPLACE`). 60 bits of entropy.
- One nit: `isUniqueViolation` (store.go:151-159) matches on the substrings "unique" or "constraint", so **any** future constraint violation (a NOT NULL, a CHECK) would be swallowed as a collision and retried 5 times before surfacing a misleading "could not generate a unique id". **Severity: MINOR.**

### B12. Badge color arithmetic: correct and timezone independent
- internal/badge/badge.go:47-58 truncates both dates to UTC midnight before subtracting, so the boundary does not move with the server's zone and `math.Round` is operating on exact multiples of 24h (no DST drift, because both operands are already UTC). `handleBadge` passes `time.Now()` in local time (handlers.go:163) but `Color` calls `.UTC()` on it, so the local zone never leaks. Same for `ageInDays` (view.go:98-102) and for `validateSubmit`'s today (validate.go:132).
- Future audit date: would give negative days and green forever, but validate.go:136 rejects any date after today UTC, so no such row can be created through the API. A row inserted by hand could. **No finding.**
- Verified 29 green / 30 amber by test and by code.

### B13. Validation: everything else I could throw at it was rejected
Verified live against a local instance (HTTP status in brackets):
- unknown field [400], body over 8192 bytes [413], `1.5` for an int field [400], `2026-02-30` [400], `2027-01-01` future [400], `"Public"` wrong case [400], `` control char in name [400], `fixed+accepted > found` [400], 6th POST in the hour [429].
- By code read and consistent with the above: number as string, negative counts, counts over 9999, `critical_open > found`, missing required field, whitespace only name (trimmed to empty, hits the 1..60 check), `javascript:`/`data:`/`file:`/`ftp:` repo URLs (scheme must be exactly `https`), repo URL when source is closed, date before 2025-01-01. Lengths are counted in **runes** for name and summary, which is the right choice.
- The only gaps are B1 (unicode format/bidi) and B2 (userinfo).

---

## 2. SPEC.md divergences

| # | SPEC | Code | Severity |
|---|---|---|---|
| D1 | SPEC.md:58 repo_url "≤ 200 chars" | validate.go:106 uses `len(repoURL)` (bytes), not runes. Inconsistent with name/summary, which use `utf8.RuneCountInString`. Harmless for real URLs, but the contract says chars. | MINOR |
| D2 | SPEC.md:62 "Log one line per POST (ip, status)" | handlers.go:119-121 and 129-131 exit via `s.serverError` with no POST log line. | MINOR |
| D3 | SPEC.md:78 `LandingData.EntryCount` | Set in handlers.go:40, never referenced by landing.html. Dead field, so the count SPEC promises is not on the page. | MINOR |
| D4 | SPEC.md:29 "No IP addresses are stored, ever" | Honored in the DB. Confirmed: `clientIP` is used only for the limiter and log lines (server.go:141, handlers.go:134). Logs do carry the IP, which SPEC.md:62 explicitly permits. | no divergence |
| D5 | SPEC.md:36 "newest approved_at first" | Implemented, but see B7: second resolution, no tie break. The contract is not wrong, the implementation just cannot honor it deterministically. | MINOR |
| D6 | SPEC.md:29 status transitions | SPEC describes statuses but never says whether `hidden` is terminal. Code lets hidden go back to approved (B6). The contract is silent, so this is a gap rather than a divergence. | see Q3 |

Everything else in the contract matched: routes, status codes, badge headers, badge text and title strings including the U+00B7 middle dot and the amber "stale" word, plate hexes, CSP values, embed layout (`web/embed.go` is exactly what SPEC.md:17 prescribes), the id alphabet and length, the counter bounds, the tier and source sets, the fixture fields, and `InstallCommands`. The no-dash rule holds: no template text node contains `-`, `–` or `—` outside ISO dates, attributes and the repo path. No `style=`, no `<script>`, no inline handler anywhere in the templates. `site.css` loads only two self hosted woff2 files and no remote resource.

---

## 3. Questions for the owner

1. **What in the record lets anyone tell a real closeout from a hand-crafted POST?** Nothing in the row does: `name`, the counts, `audit_tier` and `audit_date` are all submitter supplied, the endpoint is unauthenticated, and no provenance is stored (no plugin version, no run id, no signature, no submitting identity, not even a hash of the closeout the plugin produced). `source: "closed"` removes even the repo link. So today the only thing standing behind the badge is Matt reading the row and deciding it smells right, and the badge says `Audit 2026-09-16, 0 critical open` with the site's name on it. That is the product's whole claim. If it stays this way, the wall copy should say plainly that the numbers are self reported and the site verified the summary, not the audit. If it should not stay this way, the cheapest fix is the plugin signing the payload with a per-project key and the row storing the signature and the plugin version. Code does not contradict SPEC here, so this is a question, not a finding.
2. **Should a pending entry's badge be cacheable?** Settled that it serves (I am not re-flagging that). But it serves green, with `max-age=3600`, before any human has looked. Combined with camo (B4) a submitter can have a live green itworks.dev badge in their README minutes after POSTing, for an hour at a time, whatever Matt later decides. `Cache-Control: no-store` while pending would cost nothing and keep the settled behavior.
3. **Is `hidden` meant to be terminal?** Right now hide is reversible with one POST and leaves no history (B6). If hidden means "this was fraudulent", it should probably be one way, or at least stamped with who hid it and when.
4. **Do you want a unicode policy on `name` and `summary`?** The honest bar for a trust wall is stricter than "no control characters": reject bidi overrides and isolates, reject zero width joiners and spaces, NFC normalize, and consider confusable detection on names that collide with an existing entry. Cheap to add in `isPrintable`.
5. **Should the wall show the repo host rather than the word "repo"?** It closes B2 by design instead of by validation, and it is more informative.

---

## 4. What the tests do not prove

`go test ./internal/...` passes (badge, server, store). What that does **not** cover:

**internal/badge/badge_test.go** (213 lines, read in full): it proves the color boundary at 29/30, red beating amber, the three glyph shapes, the plate hexes and their exclusivity, the old failing hexes being gone, the "stale" word, the accessible title strings, and amber being wider than green. It does **not** prove: the SVG is well formed XML (every assertion is `strings.Contains` on a string; a malformed document would pass), that any input is escaped, that a hostile `date` argument cannot break out (no test passes anything but a clean ISO date), that the boundary holds when `now` is in a non UTC location (every case is built with `time.UTC`), or that the computed `textLength` matches any real font.

**internal/store/store_test.go** (103 lines, read in full): it proves create, get, not found, one approve, one hide, the three list queries return the right counts, `approved_at` gets set, approving an unknown id is `ErrNotFound`, and `CountPending` counts. It does **not** prove: **list ordering** (every list test has exactly one row, so `ORDER BY approved_at DESC` is never exercised), idempotency or the hide-then-approve transition, concurrent writes or the busy timeout, that migrate is safe against an existing populated database (every test starts from `t.TempDir()` with an empty file), the id collision retry path or the alphabet (only `len(id)` is checked, never the characters), or that no row is ever overwritten.

**internal/server** tests exist (server_test.go, 516 lines) but belong to lane A; I did not review their assertions.

**Nothing anywhere** tests `validateSubmit` directly against hostile unicode, and nothing renders a template with an adversarial entry and asserts the output. Both gaps are exactly where B1 and B2 live.

---

## 5. Coverage

**Read in full:** internal/server/validate.go, internal/store/store.go, internal/store/store_test.go, internal/badge/badge.go, internal/badge/badge_test.go, web/embed.go, web/templates/base.html, landing.html, wall.html, entry.html, admin.html, fixtures/seed-msp-sentinel.json, docs/SPEC.md, .itworks/LANES.md (lane B), internal/server/handlers.go, internal/server/view.go, internal/server/server.go (needed for CSP, routes and the badge headers).

**Read partially, deliberately:** web/static/site.css (930 lines) was grepped rather than read line by line, per the lane brief's "only for anything that renders user data or loads remote resources": searched for `url(`, `@import`, `content:`, `expression`, and reviewed every hit. Only two `url()` references, both to self hosted woff2 files under `/static/fonts/`; every `content:` is a static literal or a CSS counter; no remote origin, no `@import`, no data URI.

**Skipped:** internal/server/server_test.go (lane A owns it), internal/server/ratelimit.go (lane A), cmd/itworks/main.go (lane A/C), web/static/fonts/* (out of scope per LANES.md).

**Probes run:** `go test ./internal/...` (all pass). Local instance on ports 18092/18093/18094 with `ITWORKS_DB=<scratchpad>/audit/laneB.db`, `ITWORKS_ADMIN_USERS=matthew`: 11 POSTs of mutated fixtures covering injection, bidi, userinfo URL, impossible date, future date, unknown field, oversize body, float for int, count inconsistency, control character, wrong case enum, and rate limit exhaustion; two approvals; approve/approve/hide/approve idempotency; approve and hide of a non-existent id; fetched `/wall`, `/api/entries`, `/badge/{id}.svg` with headers; read the rows back with sqlite3. Server restarted three times against the same database file to exercise migrate against existing rows.

**Caveat on the scratchpad database:** `laneB.db` picked up roughly 22 pending rows at 22:10:10 that my probes did not create and that my server's log does not contain, so another agent's probe appears to share this scratchpad directory. It did not affect any finding above (all of mine are keyed to ids I created), but the row list in that file is not solely mine.

No file inside the repository was created, edited or deleted. No agents were spawned.
