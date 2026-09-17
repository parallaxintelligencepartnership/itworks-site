# Lane B verification (records and rendering, lens real-data)

Verifier did not write the lane report. Repo /Users/matthew/parallax-private/Projects/itworks-site @ 5a87acd, read only: **no repo file was created, edited or deleted.** All test code lives outside the repo and was run with `go test -overlay`.

Artifacts in this directory:
- `overlay/verify_unicode_test.go` (package server), `overlay/verify_badge_test.go` (package badge), `overlay/overlay.json` — run with
  `go test -overlay <audit>/overlay/overlay.json -run TestVerify ./internal/server/ ./internal/badge/`
- `verifyB.db`, `verifyB.log` — fresh local instance on :18092, `ITWORKS_ADMIN_USERS=matthew`. Server killed after the probes.

---

## B1. Bidi and zero-width characters pass validation and render raw — **CONFIRMED, IMPORTANT**

**Live reproduction.** POST (201 Created, id `qc7wuucwzg6h`), then approved:

```json
{"name":"paypal‮ gro.live​‍﻿","summary":"bidi and zero width probe",
 "source":"public","repo_url":"https://example.com/a","audit_tier":"audit",
 "audit_date":"2026-09-16","found":5,"fixed":1,"accepted":1,"critical_open":0}
```

Stored (sqlite3 on verifyB.db): `paypal<U+202E> gro.live<U+200B><U+200D><U+FEFF>`.
Rendered `/wall`, hexdump of the name cell:

```
<h3 class="name"><a href="/e/qc7wuucwzg6h">paypal e2 80 ae 20 67 72 6f 2e 6c 69 76 65 e2 80 8b e2 80 8d ef bb bf</a></h3>
                                                  ^U+202E   " gro.live"        ^ZWSP    ^ZWJ     ^BOM
```

The four bytes sequences are emitted verbatim (`html/template` correctly escapes `<`, `&`, `"` — it does not touch format characters, which is correct behavior for the escaper and therefore a validation problem, not a template problem). **Stored value is `paypal‮ gro.live`; displayed value is `paypal evil.org`** — the U+202E reverses the tail. The admin queue renders through the same `{{.Name}}` (admin.html:3), so the approver approves the reversed string, not the stored one.

**Unit reproduction** (`TestVerifyB1_RejectFormatAndBidiChars`, fails on current code):
```
U+202E RLO: accepted "paypal‮ gro.live", want rejection   (and same for summary)
U+200B ZWSP, U+200D ZWJ, U+FEFF BOM, U+2066 LRI: all accepted
```
Root cause: `isPrintable` (internal/server/validate.go:181-190) rejects only `unicode.IsControl`. Every character above is category **Cf** (format), not Cc, so it passes.

**Smallest correct fix.** In `isPrintable`, additionally reject `unicode.Is(unicode.Cf, r)` (covers U+200B–U+200F, U+202A–U+202E, U+2066–U+2069, U+FEFF, U+061C) plus `unicode.Is(unicode.Cs, r)` and `unicode.Is(unicode.Co, r)`; optionally NFC-normalize first. Do **not** reject by a hand-rolled range list — Cf is the right category and it is one predicate. Note U+00A0 is Zs, not Cf; if non-breaking padding matters, fold Zs (other than U+0020) to a space during trim rather than rejecting.
**Must keep working:** `TestVerifyB1_AllowLegitimateNames` passes today and must keep passing — `café`, `ma` + U+0301 (combining acute, category Mn), `日本語ツール`, `shipit 🚀`, `Ünïcödé-tool`. A blanket "ASCII only" or "printable per unicode.IsPrint" screen would break the combining-mark and emoji cases (`unicode.IsPrint` accepts U+200B anyway, so it is not the fix either).

**Fix spec.** File `internal/server/validate.go`, func `isPrintable`. Test that must fail before and pass after: `TestValidateSubmit_RejectsFormatCharacters` with inputs `"paypal‮ gro.live"`, `"pay​pal"`, `"pay‍pal"`, `"paypal﻿"`, `"paypal⁦x⁩"` against both `name` and `summary`, plus an allow-list case with the five legitimate names above. Must not change: rune-based length counting for name/summary, the trim-then-length order, the plain-English error message style, the existing control-character rejection.

---

## B2. `repo_url` with userinfo accepted and rendered as the "repo" link — **CONFIRMED, IMPORTANT**

**Live reproduction.** POST `"repo_url":"https://github.com@evil.example.com/x"` → 201, id `xkv7igq4myxf`; approved; `/wall` contains exactly:

```html
<p class="src"><b>public</b> <a href="https://github.com@evil.example.com/x">repo</a>
```

The link text is the word `repo`; the host is never shown. A visitor doing the check the wall invites is sent to `evil.example.com` with a status bar that reads GitHub-first.

**Unit reproduction** (`TestVerifyB2_RejectUserinfoInRepoURL`, fails on current code): both `https://github.com@evil.example.com/x` and `https://github.com:x@evil.example.com/x` are accepted. Root cause: validate.go:98-113 checks `u.Scheme == "https"` and `u.Host != ""` and never looks at `u.User`.

**Smallest correct fix: `u.User != nil` → reject.** `url.Parse` already populates `u.User` for both forms, so it is a one-line check. Do **not** add a host allowlist: the same test asserts `https://gitlab.com/a/b`, `https://codeberg.org/a/b` and a self-hosted `https://git.parallax.example:3000/a/b` still validate (they do today and must continue to). Optionally also reject a host containing Cf characters and reject `u.Opaque != ""`; the userinfo check alone closes the reported attack. Question 5 in the report (render the host instead of the word "repo") is a complementary design fix, not a substitute.

**Fix spec.** File `internal/server/validate.go`, func `validateSubmit`, the repo_url block. Test: `TestValidateSubmit_RejectsRepoURLUserinfo` — reject the two userinfo URLs, accept the four legitimate ones. Must not change: scheme must stay exactly `https`, empty repo_url stays legal, `repo_url` must stay empty when source is `closed`, and the `javascript:`/`data:`/`file:`/`ftp:` rejections.

---

## B3. No `rel` on visitor-supplied links — **CONFIRMED, ADVISORY**

`grep -rn 'rel=' web/templates/` returns exactly one hit, `base.html:8` (the stylesheet). The three visitor-controlled links (base.html:39, entry.html:17, admin.html:4) are plain dofollow. The finding is accurate. Severity lowered from the report's IMPORTANT to **ADVISORY** under the real-data rubric: nothing about the record or its rendering is wrong and no viewer is misled — the harm is SEO/incentive, which the rubric's "otherwise" bucket covers. It is still worth doing and costs one attribute.

**Fix spec.** Files `web/templates/base.html:39`, `entry.html:17`, `admin.html:4`: add `rel="nofollow ugc noopener noreferrer"` to the repo anchors only (not to `/e/{{.ID}}` internal links). Test: extend a template render test to assert the rendered repo anchor contains `rel="nofollow ugc`. Must not change: no `target="_blank"` is introduced, internal links keep no rel.

---

## B4. Hiding an entry does not retract the cached badge — **CONFIRMED (core), PARTIAL on the camo purge claim), IMPORTANT**

**Live reproduction**, entry `qc7wuucwzg6h`:
```
GET /badge/qc7wuucwzg6h.svg  -> HTTP 200, Cache-Control: public, max-age=3600
POST /admin/entries/qc7wuucwzg6h/hide -> 303
GET /badge/qc7wuucwzg6h.svg  -> HTTP 404
```
Origin retracts instantly; every shared cache and every already-fetched copy keeps the last green SVG for up to 3600 s (handlers.go:169). Confirmed.

**Correction to the report.** The report states camo "offers no purge to a third party origin". Camo does expose a `PURGE` method on the camo URL, and there is a small ecosystem of tools built on it (`hobbyquaker/camo-purge`, `kevincobain2000/action-camo-purge`, `sbts/github-badge-cache-buster`). So a purge path exists — but it is per-image, it must be issued against the *camo* URL (which the origin does not know; it is derived from the README owner's rendered HTML), and GitHub users report camo not honoring `Cache-Control` reliably ([community discussion 156383](https://github.com/orgs/community/discussions/156383)). Practical effect for itworks.dev is what the report says — Matt cannot retract a badge from a README he does not control — but the mechanism claim is **PARTIAL**: purge is not absent, it is not reachable by the origin.

**What max-age makes a hide visible in an acceptable window.** `max-age=3600` means up to an hour of a green badge for a record Matt has ruled fraudulent. Shields.io's own dynamic badges sit around 300 s and that is the de facto floor caches tolerate. Recommendation: `public, max-age=300` for approved entries (worst case 5 minutes, ~12 origin fetches/day/badge — nothing at this scale), and `no-store` while `status = pending`, which also closes report question 2 (a green badge live in a README minutes after an unreviewed POST).

**Open design question, not decided here:** should a hidden badge serve a grey "withdrawn" badge (200) instead of 404? A 404 leaves a broken-image icon that a reader may read as "site down" rather than "claim withdrawn", and it gives caches nothing fresh to replace the green with; a grey 200 replaces the stale green with an explicit retraction and is self-explanatory in a README. It also means a hidden entry still renders something bearing the site's name. **This is Matt's call — flagged, not decided.**

**Fix spec (the cache part only).** File `internal/server/handlers.go`, func `handleBadge` (line 169): set `Cache-Control: no-store` when `e.Status == store.StatusPending`, else `public, max-age=300`. Test: `TestHandleBadge_CacheHeaders` — pending id → `no-store`; approved id → `max-age=300`; hidden id → still 404. Must not change: the badge still serves 200 for pending (settled), the `Content-Type`, the `Content-Security-Policy: default-src 'none'`, and `X-Content-Type-Options: nosniff` (all three verified present live).

---

## B5. `Render` builds the SVG with `fmt.Sprintf` and escapes nothing — **CONFIRMED as latent (PARTIAL: not reachable today), ADVISORY**

**Trace of every interpolated value** (internal/badge/badge.go:133-155), from `handleBadge` (handlers.go:163-164):
| interpolated | origin | validation |
|---|---|---|
| `title`, `rightText` | `fmt.Sprintf` over `date` + `criticalOpen` + `color` | `date` = `e.AuditDate`, validate.go:127-137: `time.Parse("2006-01-02")` **and** round-trip `Format == input`, so it is exactly 10 chars from `[0-9-]`; `criticalOpen` is an `int` 0..9999; `color` is a `badge.Color` return value, one of three constants |
| `totalW`, `leftW`, `badgeHeight`, x/y/textLength | computed `%d`/`%.1f` from the above | numeric |
| `stateHex`, `hexLabel`, `hexHairline`, `hexText`, `rightTextHex` | `hexFor`/`textHexFor` over the closed color set | closed set |
| `glyph(color, cx)` | closed switch over the three colors | closed set |
| `leftLabel`, `fontFamily`, `fontSize`, `fontWeight` | package constants | n/a |
**No visitor-controlled string reaches the SVG today.** Verdict on exploitability: the report is right that this is latent only, so **PARTIAL / ADVISORY**, not IMPORTANT.

**But the latent defect is real and reproducible** (`TestVerifyB5_RenderIsWellFormedXML`, fails on current code):
```
date "]]></text><script>alert(1)</script><text>": XML syntax error: unescaped ]]> not in CDATA section
date "\"><g onload=\"x":                          XML syntax error: unescaped < inside quoted string
```
A clean ISO date passes. So the moment anything less constrained than `date` is interpolated — `Name` being the obvious next feature, and the alt text already pairs name with badge — the SVG stops being well-formed XML and is served as a same-origin document from itworks.dev with only the badge CSP in the way.

**Fix spec.** File `internal/badge/badge.go`, func `Render` (and `glyph`): wrap every interpolated string in an `xmlEscape(s string) string` helper (escape `& < > " '`), or rebuild via `text/template` with `html/template`-grade escaping. Test: keep `TestVerifyB5_RenderIsWellFormedXML` as `TestRender_WellFormedXMLForHostileInput` with the two hostile date strings above plus a clean date; it must fail before and pass after. Must not change: the exact badge text including the U+00B7 middle dot, the amber "stale" word, the `<title>` strings, the plate hexes, the three glyph shapes, and the computed `textLength` values (escaping a clean ISO date is a no-op, so no existing badge_test.go assertion should move).

---

## B6. Approve has no source-state precondition; hide→approve silently republishes — **CONFIRMED, IMPORTANT**

**Live reproduction**, entry `qc7wuucwzg6h`, approved at `2026-09-16T22:15:47Z`:
```
POST /admin/entries/qc7wuucwzg6h/hide     -> 303   (badge now 404, off the wall)
POST /admin/entries/qc7wuucwzg6h/approve  -> 303
sqlite3: qc7wuucwzg6h|approved|2026-09-16T22:16:06Z
```
Back on the wall, and **`approved_at` was rewritten** — the re-approval moved it ahead of `xkv7igq4myxf` (still `22:15:47Z`) in `ORDER BY approved_at DESC`, confirmed in the rendered `/wall`. `.schema entries` has **no `hidden_at` and no `hidden_reason` column**: the fact that the entry was ever hidden exists only in a log line (handlers.go:277-283). Moderation history is unrecoverable, which is the report's claim and it holds. `store.Approve` (store.go:227-234) is `UPDATE entries SET status=?, approved_at=? WHERE id=?` with no status predicate.

**Fix spec.** File `internal/store/store.go`, funcs `Approve` and `Hide`. (a) `Approve`: preserve `approved_at` when already set — `approved_at = COALESCE(approved_at, ?)` — so re-approval cannot reorder the wall; (b) add `hidden_at TEXT NULL` (and optionally `hidden_reason`) via `ALTER TABLE ... ADD COLUMN` guarded by a column check, stamped by `Hide`; (c) decide whether `hidden` is terminal — see the question below. Test: `TestApprove_HideThenApprovePreservesApprovedAt` — create, approve, record `approved_at`, hide, approve, assert `approved_at` unchanged and `hidden_at` non-null; and a two-row ordering test asserting the re-approved row did not jump. Must not change: approve/hide stay idempotent and non-destructive (verified: approve, approve, hide, approve all 303, no row lost), an unknown id still returns 404 via `checkRowsAffected`, and no `INSERT OR REPLACE` appears anywhere.

**Question for the owner (report Q3, unresolved and I cannot resolve it from the code):** SPEC.md:29 lists `pending|approved|hidden` and never says whether `hidden` is terminal. Making it terminal is a product decision about what "hidden" means (fraud vs. temporary takedown). **Owner's call.**

---

## B7. Wall order ties are unstable — **PARTIAL, ADVISORY**

The premise is confirmed: two entries approved in the same second both carry `approved_at = 2026-09-16T22:15:47Z` (RFC3339, second resolution, store.go:228), and `ORDER BY approved_at DESC` (store.go:205) has no tie break, so SQLite defines no order. The *consequence* the report states ("can swap between page loads") did **not** reproduce: five consecutive `/wall` fetches returned an identical order, because with a single connection SQLite walks the same scan order every time. So it is an undefined contract that a current implementation detail happens to make deterministic — real, but it does not today produce a wrong page, hence ADVISORY rather than MINOR-as-bug.

**Fix spec.** File `internal/store/store.go:205`: `ORDER BY approved_at DESC, created_at DESC, id DESC`. Test: `TestListApproved_OrderWithTiedApprovedAt` — two rows with identical `approved_at`, assert a deterministic documented order. Must not change: "newest approved_at first" stays the primary key of the sort (SPEC.md:36); `ListPending`/`ListHidden` keep `created_at ASC`. Note this test is new ground — every existing list test in store_test.go has exactly one row, so ordering is untested today.

---

## B8. Pending cap is check-then-act — **CONFIRMED by trace, ADVISORY**

handlers.go:118-132 is `CountPending()` then, separately, `Create()`, with no guard between them; the cap is therefore advisory under concurrency. Mitigation the report cites is correct: `db.sql.SetMaxOpenConns(1)` (store.go:75) serializes the two statements, so overshoot is bounded by concurrent in-flight requests and, critically, **nothing is dropped or overwritten** — the stated invariant holds. Not reproduced live (would need a race harness for a non-consequence). ADVISORY, and the report's own framing ("design note") is right.

---

## B9 / D2. 500s on the count and create paths are not logged as POST lines — **CONFIRMED, ADVISORY**

`serverError` (handlers.go:306-309) is exactly:
```go
s.logger.Printf("server error: %v", err)
http.Error(w, "internal server error", http.StatusInternalServerError)
```
No ip, no `status=`. Both `s.serverError` exits on the POST path (handlers.go:119-121 count failure, 129-131 create failure) therefore produce no `POST /api/entries ip=... status=...` line, while every other exit goes through `finishPost` (handlers.go:141-143) which does log one. SPEC.md:62 says "Log one line per POST (ip, status)". The divergence is real; the code is wrong against the SPEC and the SPEC is the intended contract (an unlogged 500 is the one you most need logged). Severity ADVISORY: observability only, no record or rendering is wrong.

**Fix spec.** File `internal/server/handlers.go`, func `handlePostEntries`: replace both `s.serverError(w, err)` calls with a variant that emits the POST line (e.g. `s.logger.Printf("POST /api/entries ip=%s status=500", ip)` before `s.serverError`, or give `finishPost` a 500 path). Test: `TestPostEntries_LogsLineOn500` with an injected failing store, asserting the captured log contains `POST /api/entries ip=` and `status=500`. Must not change: the 500 body stays the generic `internal server error`, and bodies are still never logged (SPEC.md:62).

---

## B10 / B11 / B12 / B13 — **REFUTED as findings (i.e. the report's "no finding" verdicts hold)**

- **B10 schema/WAL/pool:** re-verified by code read — `CREATE TABLE IF NOT EXISTS` + `CREATE INDEX IF NOT EXISTS`, no `DROP`/`ALTER`. A fresh `verifyB.db` was created, written, read back and served without error. No finding.
- **B11 id generation:** 256 is an exact multiple of 32, so `idAlphabet[int(v)%32]` is unbiased — the report's arithmetic is right and the usual modulo-bias claim does **not** apply. The `isUniqueViolation` nit is confirmed by reading store.go:151-159: it matches the substrings `"unique"` **or** `"constraint"` lowercased, so any future NOT NULL/CHECK violation is swallowed and retried five times before surfacing "could not generate a unique id". Real, latent, **ADVISORY**; fix is to narrow the match to `"unique constraint"`. No test exists for the retry path.
- **B12 badge color arithmetic:** both operands truncated to UTC midnight before subtraction; `Color` calls `.UTC()` on the caller's local `time.Now()`, so the zone cannot leak. Timezone independence holds. No finding.
- **B13 validation coverage:** the report's rejection list is consistent with the code I read; the only accepted-but-should-not-be inputs are B1 and B2, both now reproduced. No additional gap found.

---

## SPEC divergences

**D1 — repo_url length is bytes, not chars. CONFIRMED, ADVISORY.**
SPEC.md:58: `repo_url | ... parses with net/url, scheme https, non empty host, ≤ 200 chars`.
validate.go:106: `if len(repoURL) > maxRepoURLLen {`.
`len()` is bytes; name and summary use `utf8.RuneCountInString`. Reproduced (`TestVerifyD1_RepoURLLengthInRunes`): a 150-rune, 280-byte URL is rejected with "repo url is too long" although SPEC allows 200 chars. **Intended contract: the SPEC** — it says chars, and the two sibling fields already count runes, so the code is the outlier. Fix: `utf8.RuneCountInString(repoURL)` at validate.go:106; test as above; must not change the 200 limit itself.

**D2 — POST log line missing on the 500 paths. CONFIRMED, ADVISORY.** Same item as B9; SPEC.md:62 is the intended contract. Fix spec under B9.

**D3 — `LandingData.EntryCount` is dead. CONFIRMED, ADVISORY.**
SPEC.md:78 declares `type LandingData struct { Title string; EntryCount int; ... }`; handlers.go:40 sets `data.EntryCount = len(approved)`. `grep -rn EntryCount web/templates/ internal/` returns only view.go:33 (the declaration) and handlers.go:40 (the assignment) — **zero template references**. So the count SPEC implies the landing page carries is computed and thrown away. **Which side is intended is not determinable from the code**: SPEC declares the field but never says the landing page must render a count, so either "render it" or "drop the field" satisfies the written contract. **Question for the owner.** Whichever way: if it renders, it must render `len(approved)` (approved only, not pending), since that is what is computed today.

**D4 — "No IP addresses are stored, ever". No divergence, verified.** `.schema entries` has no IP column; `clientIP` feeds only the limiter and the log lines, which SPEC.md:62 explicitly permits.

**D5 — "newest approved_at first". No divergence in the contract; see B7.** Implemented as written; the second-resolution tie is an implementation gap, not a contract mismatch.

**D6 — status transitions.** SPEC.md:29 names the three statuses and is silent on whether `hidden` is terminal; the code allows hidden→approved (B6). This is a **gap in the SPEC, not a divergence** — an owner question (Q3), recorded under B6.

---

## Verdict table

| Suspicion | Verdict | Severity |
|---|---|---|
| B1 bidi / zero-width chars reach the wall | CONFIRMED | IMPORTANT |
| B2 userinfo in repo_url renders as the repo link | CONFIRMED | IMPORTANT |
| B3 no `rel` on visitor links | CONFIRMED | ADVISORY |
| B4 hide does not retract the cached badge | CONFIRMED (PARTIAL on "camo has no purge") | IMPORTANT |
| B5 `fmt.Sprintf` SVG, nothing escaped | PARTIAL (latent; not reachable today) | ADVISORY |
| B6 hide→approve republishes and rewrites `approved_at` | CONFIRMED | IMPORTANT |
| B7 wall order ties | PARTIAL (undefined, but deterministic in practice) | ADVISORY |
| B8 pending cap check-then-act | CONFIRMED (bounded by MaxOpenConns=1) | ADVISORY |
| B9 500s on POST not logged as POST lines | CONFIRMED | ADVISORY |
| B10 schema / WAL / pool | REFUTED (no finding stands) | — |
| B11 id generation (incl. modulo) | REFUTED; `isUniqueViolation` substring nit CONFIRMED | ADVISORY |
| B12 badge colour arithmetic / timezone | REFUTED (no finding stands) | — |
| B13 rest of validation | REFUTED (no further gap) | — |
| D1 repo_url length in bytes not runes | CONFIRMED (SPEC is the contract) | ADVISORY |
| D2 POST log line on 500 paths | CONFIRMED (SPEC is the contract) | ADVISORY |
| D3 `LandingData.EntryCount` dead | CONFIRMED; intent undeterminable | ADVISORY / owner question |
| D4 no IPs stored | REFUTED (no divergence) | — |
| D5 newest approved_at first | REFUTED as a divergence (see B7) | — |
| D6 is `hidden` terminal | SPEC gap, not a divergence | owner question |

Nothing reached CRITICAL: no fake data is presented as verified counts, no public trust signal is wrong without a bounded cache window, and no data is lost. B1, B2, B4 and B6 are the four that weaken the record or the rendering for real.
