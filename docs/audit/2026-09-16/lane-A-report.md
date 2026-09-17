# Lane A review: Edge and identity (lens security-auth)
Repo: /Users/matthew/parallax-private/Projects/itworks-site @ 5a87acd. Read-only. All probes on a local instance on ports 18081/18082.

## 1. Suspicions

### A1. CRITICAL - a percent-encoded /admin path is served as /admin by the app but is not /admin to Traefik, so forward-auth is skipped and the app's only remaining check is a header the caller sets
- Where: internal/server/server.go:98-100 (routes), server.go:121-143 (requireAdmin), docker-compose.yml:21-34 (router rules).
- The app's entire trust chain is: Traefik routes anything under PathPrefix(`/admin`) through `authentik-auth@file`, which sets X-Authentik-Username; the app then believes that header. Nothing in the app verifies the request actually traversed forward-auth, and the public router (docker-compose.yml:21-25) has no middleware that strips or overwrites a client-supplied X-Authentik-Username. Any spelling of the path that Traefik does not classify as /admin but Go's ServeMux does is a complete admin takeover.
- Proven on the app side: `POST /%61dmin/entries/{id}/approve` with a self-supplied `X-Authentik-Username: matthew` and `Origin: <same host>` returns **303 and approves the entry**. Go's ServeMux matches on the decoded path, so `%61` -> `a` and the pattern `POST /admin/entries/{id}/approve` matches. The same trick reaches `/api/entries` (`/%61pi/entries` was rate limited by the same bucket, i.e. the handler ran).
- The Traefik half is the part I could not execute here: Traefik v3's Path/PathPrefix matchers work on the raw, still-encoded path, so `/%61dmin/...` does not match PathPrefix(`/admin`) and falls through to the public router (priority 1, no auth middleware). If that is how the deployed Traefik behaves, this is a full unauthenticated approve/hide primitive: a stranger can approve their own pending entry and publish a green badge, or hide anyone else's.
- Wrong outcome: a stranger changes stored record outcomes (approve/hide) with no authentication.
- Severity: CRITICAL (stranger acts as the admin and changes stored permission/record outcomes). If the verifier establishes that the deployed Traefik version decodes before matching, downgrade to IMPORTANT and keep the finding, because the app still has no independent proof that forward-auth ran.
- Reproduce:
  1. `ITWORKS_DB=<tmp>/a.db ITWORKS_BASE_URL=http://localhost:18081 ITWORKS_ADDR=:18081 ITWORKS_ADMIN_USERS=matthew go run ./cmd/itworks &`
  2. `ID=$(curl -s -H 'Content-Type: application/json' -d '{"name":"X","summary":"s","source":"closed","audit_tier":"audit","audit_date":"2026-01-01","found":0,"fixed":0,"accepted":0,"critical_open":0}' localhost:18081/api/entries | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')`
  3. `curl -s -o /dev/null -w '%{http_code}\n' --path-as-is -X POST -H 'X-Authentik-Username: matthew' -H 'Origin: http://localhost:18081' "http://localhost:18081/%61dmin/entries/$ID/approve"` -> 303, and the entry now appears in `GET /api/entries`.
  4. Traefik half: on a Traefik v3 instance with these two routers, request `/%61dmin` with `--path-as-is` and confirm which router serves it (Traefik access log `RouterName`).
- Observed non-bypasses (for the verifier's benefit): `//admin/...`, `/./admin/...`, `/admin/../admin/...` all return 307 to the canonical `/admin/...` (which re-enters Traefik and hits forward-auth); `/ADMIN/...`, `/admin/.../approve/`, `/admin/.../approve%2e` all 404. Only the percent-encoding of a letter inside `admin` works.

### A2. IMPORTANT - the same-origin check ignores the scheme and accepts a scheme-less or userinfo-bearing Origin
- Where: internal/server/server.go:135-139. `url.Parse(origin)` then only `u.Host != r.Host` is compared.
- Proven: against `Host: localhost:18081`, these Origins are all accepted and the approve executes (303): `http://localhost:18081`, `https://localhost:18081`, `ftp://localhost:18081`, `http://user:pw@localhost:18081`, `http://localhost:18081/evil`, `//localhost:18081`. Correctly rejected (403): `http://evil.com`, `null`, `localhost:18081` (no scheme marker, parses as a path), `HTTP://LOCALHOST:18081` (case - Go does not lowercase the host here), `http://localhost` (port mismatch), missing Origin.
- Wrong outcome: any content that can be made to run at `http://itworks.dev` (an active network attacker before HSTS is pinned, a stray plaintext vhost, or a non-https scheme handler) can forge a same-origin CSRF against approve/hide while Matt's Authentik session cookie is live. HSTS (docker-compose.yml:40-41) mitigates but does not remove it, and the check as written also silently accepts `null` variants of the form `//host`.
- Severity: IMPORTANT.
- Reproduce: start the instance as above, then `curl -X POST -H 'X-Authentik-Username: matthew' -H 'Origin: ftp://localhost:18081' localhost:18081/admin/entries/$ID/approve` -> 303.
- Note also that `HTTP://LOCALHOST:18081` denies. Browsers send a lowercase scheme and host so this does not break real traffic, but it shows the comparison is a raw string compare with no normalisation on either side.

### A3. IMPORTANT - with ITWORKS_TRUST_PROXY=1 (the production setting) X-Real-Ip is trusted absolutely, so one caller who can reach the container directly has an unlimited POST quota
- Where: internal/server/server.go:147-152; docker-compose.yml:9 sets `ITWORKS_TRUST_PROXY: "1"`.
- Proven (trust_proxy=1 instance on :18082): 12 consecutive POSTs rotating `X-Real-Ip: 10.0.0.1..12` all returned 201; 7 POSTs with a fixed forged value gave 201x5 then 429. So the limiter keys entirely on an attacker-chosen string. `X-Forwarded-For` is never read (invariant 3's second half holds - only X-Real-Ip matters).
- With trust_proxy unset the header is ignored entirely: 7 POSTs carrying rotating `X-Forwarded-For` gave 201x5 then 429 on the RemoteAddr key. Invariant 3's first half holds.
- Wrong outcome: the container sits on the shared external network `dmz-infrastructure_dmz-public` (docker-compose.yml:13-14, 46-48) with no published host port but reachable on :8080 by every other container on that network. Anything on that network, or any future path that reaches the app other than through Traefik, gets an unbounded submission rate and can fill the 500-entry pending queue (handlers.go:118-126), which then 503s every honest submitter.
- Severity: IMPORTANT. There is no check that the request came from the proxy (no trusted-proxy CIDR), which is what the invariant "only the proxy's value" actually requires.
- Reproduce: `ITWORKS_TRUST_PROXY=1 ... go run ./cmd/itworks &` then `for i in $(seq 1 12); do curl -s -o /dev/null -w '%{http_code} ' -H 'Content-Type: application/json' -H "X-Real-Ip: 10.0.0.$i" -d '<valid payload>' localhost:18082/api/entries; done` -> twelve 201s.

### A4. IMPORTANT - the limiter map is never pruned and, under trust_proxy, its keys are arbitrary attacker-supplied strings of arbitrary length
- Where: internal/server/ratelimit.go:11-49. `l.hits[ip]` is written on every call and no code path ever deletes a key or sweeps expired entries; there is no janitor goroutine anywhere in the package (`grep -n delete\|Sweep` finds nothing).
- Proven: `X-Real-Ip:` set to a 3000-character string is accepted and becomes a map key (the POST returned 201). Combined with A3, one client can create unbounded distinct keys, each holding the key string plus a `[]time.Time`, with no eviction until the process restarts.
- Wrong outcome: memory exhaustion / OOM of the single container by a remote caller; the wall, badges, and the admin page all go down with it. Even in the honest case, one key per distinct visitor IP accumulates forever.
- Severity: IMPORTANT (denial of service, not disclosure). Fix shape: cap the key to a parsed `net.IP`, and drop keys whose slice is empty after the cutoff filter.
- Reproduce: as A3 but rotate the header value over a large range and watch RSS; or read ratelimit.go and confirm no `delete(l.hits, ...)` exists.

### A5. IMPORTANT - the badge is served with `Cache-Control: public, max-age=3600`, which blunts Hide as a takedown control
- Where: internal/server/handlers.go:169 (badge), against handlers.go:157 (hidden -> 404).
- The badge is the product's trust signal and Hide is the only lever Matt has when an entry turns out to be fraudulent. A badge embedded in a README is fetched through GitHub's image proxy, which honours and often extends cache lifetimes; the app's own header invites an hour of continued green after Hide, and third-party caches can hold it longer.
- Wrong outcome: Matt hides a fraudulent entry and the green badge keeps rendering for third parties. That is the exact failure the product cannot afford.
- Severity: IMPORTANT.
- Reproduce: fetch `/badge/{id}.svg` (200, `Cache-Control: public, max-age=3600`), hide the entry, fetch again from a cache-respecting client - the cached copy is still served for up to an hour.
- Note: I am not re-flagging that pending entries get a badge (settled). This is about the cache lifetime attached to it.

### A6. ADVISORY - approve and hide have no state precondition, so hidden entries can be resurrected and re-approval re-orders the wall
- Where: internal/store/store.go:227-244 (`UPDATE entries SET status=? ... WHERE id=?`), handlers.go:247-275.
- Proven: approve -> hide -> approve on the same id all return 303, and the entry is back on the wall. Re-approving an already approved entry returns 303 and re-stamps `approved_at`, which is the wall's sort key (`ORDER BY approved_at DESC`), so a repeated approve silently promotes an old entry to the top of the wall.
- There is no transaction and no read-check-write, so a concurrent approve and hide is last-writer-wins with no error to either caller. Both are admin-only today, so this is not a stranger-reachable defect; it is a correctness gap that becomes one the moment A1 is exploitable.
- Severity: ADVISORY on its own; treat as part of A1's blast radius.

### A7. ADVISORY - the pending cap is a read-then-write with no transaction
- Where: handlers.go:118-132. `CountPending()` then `Create()` are separate statements. Concurrent POSTs can each read 499 and all insert, so the 500 cap is soft. Low impact (SQLite serialises writers and the limiter throttles), but it is a state-read-before-commit.

### A8. ADVISORY - Content-Type is matched with HasPrefix, so `application/jsonwhatever` passes
- Where: handlers.go:94-98. `strings.HasPrefix(ct, "application/json")` accepts `application/jsonfoo` and any suffix. Harmless with a strict decoder behind it, but it is not the "exact" check the invariant asks for; parse the media type instead.

### A9. ADVISORY - private pages carry no Cache-Control, and admin usernames plus visitor IPs are logged
- `GET /admin` returns 200 with Content-Type, CSP, nosniff, Referrer-Policy and X-Frame-Options, and **no `Cache-Control: no-store`** (confirmed by header dump). Same for `/e/{id}` and `/api/entries`. The admin page lists every pending and hidden entry; nothing should be allowed to store it.
- handlers.go:134, 144 log `ip=<client ip>` on every POST, and handlers.go:282 logs the admin username on every admin action. Lane C's invariant says logs carry no visitor IPs beyond what PROJECT.md allows - worth reconciling. No payload bodies are logged (good), and server.go:146 correctly notes the IP is never persisted to the database.

### Invariants that held under probing
- Invariant 1: `/e/{id}` 404s for pending and hidden; `/api/entries`, `/wall` and the landing preview all come from `ListApproved()` only; `/badge` 404s for hidden. No unauthenticated route leaks pending existence other than the settled pending badge.
- Invariant 4: the limiter is consumed before the body is touched (handlers.go:87, ahead of the content-type check and the decode) and cannot be dodged by `/api/entries/`, `//api/entries`, `/API/entries`, `/api/./entries` (404 or 307 to the canonical path) nor by any content-type variant. `/%61pi/entries` reaches the same bucket.
- Invariant 6: `GET /admin/entries/{id}/approve` returns 405, not a fallthrough. Empty `{id}` 307s to the canonical path rather than matching. `/e/%2e%2e%2fadmin`, `/e/x%00`, `/badge/%2e%2e%2f...svg` all 404. The font handler's `ContainsAny(name, "/\\")` and `..` guard holds against percent-encoded traversal.
- Invariant 8: MaxBytesReader is installed before the decode (handlers.go:100-101), the decoder sets DisallowUnknownFields and rejects trailing data (validate.go:50-58), the body is read once, and the 413 path is distinguished from the 400 path.
- Invariant 2 parsing: `parseAdminUsers` (main.go:30-39) trims spaces and drops empties, so `"matthew, , "` yields exactly `matthew`; `New` re-filters empty strings, so an empty allowlist denies everything (confirmed by test and by probe). Matching is exact and case-sensitive: `Matthew`, `MATTHEW`, `matthew,x` and empty all 403. Leading/trailing spaces in the header value are stripped by Go's own header parser before the comparison, which is correct behaviour, not a bypass.
- Invariant 5 label reading: the admin router has `priority=10` against the public router's `priority=1`, and PathPrefix(`/admin`) covers every subpath, so the intended ordering is explicit and correct. Both routers are `websecure` only. No label exposes a route the app does not guard. The gap is A1 (spellings Traefik and Go disagree about) and the fact that the public router does not neutralise X-Authentik-Username.

## 2. Questions for the owner
1. **Forgery of someone else's project (invariant 7).** Nothing in the submit path proves the submitter controls the repo. A stranger can POST `name: "kubernetes"`, `repo_url: https://github.com/kubernetes/kubernetes`, `found 40 / fixed 40 / critical_open 0`, and if approved the service hosts a green badge for a project that never ran itworks. At approval time the admin page (web/templates/admin.html) shows Matt only the submitted name, summary, source, repo link, tier, date and the four counters - all attacker-supplied, none corroborated, and no submission timestamp, no submitting IP, and no indication that two entries claim the same repo. The code does not fail its own stated contract here, so this is a design question: what evidence should approval rest on? Options worth weighing: require a proof-of-control step (a file or a tag in the repo the service fetches), show the admin page the created_at and any prior entries for the same repo_url, or state on the wall and the badge page that the claim is self-reported and admin-reviewed only.
2. **Is the limiter meant to be an abuse control or a politeness control?** As built (A3, A4) it is the latter. If it is meant as an abuse control it needs a trusted-proxy CIDR and a bounded map.
3. **Should the app be independently satisfiable that forward-auth ran?** A shared secret header set by the Authentik middleware, or refusing any request whose path does not equal its own cleaned form, would close A1 regardless of how Traefik's matcher spells things.
4. **Should Hide be irreversible, and should approve refuse an already-approved or hidden entry?** See A6; today both are unconditional UPDATEs.
5. **Is the container being reachable on :8080 by every other container on `dmz-infrastructure_dmz-public` intended?** That network is what makes A3 and a direct header-forging path reachable at all.

## 3. What the tests do not prove
The suite is green (`go test ./internal/server` ok, `go vet` clean) and covers the happy paths and the obvious denials well. It does not prove:
- **Anything about path spelling.** Every admin test uses the literal, canonical `/admin/entries/<id>/approve`. Nothing exercises `%61dmin`, `//admin`, a trailing slash, mixed case, or a dot segment. A1 is invisible to the suite.
- **Anything about Traefik.** The labels in docker-compose.yml are never asserted. The suite tests the app in isolation, where the header guard is the only guard, so it can never catch a routing mismatch between Traefik's matcher and Go's.
- **That a spoofed identity header is rejected.** `TestAdminGuardUserNotInAllowlist403` proves a wrong username is refused; no test proves that a *correct* username supplied by the client rather than by forward-auth should be refused. The suite therefore encodes the spoofable design as correct.
- **Origin beyond one cross-origin case.** `TestAdminGuardCrossOrigin403` uses `http://evil.example.com` only. No test covers scheme mismatch, `null`, `//host`, userinfo, port variants, or a missing Origin on GET vs POST. A2 is invisible.
- **Anything about proxy trust.** No test sets `TrustProxy: true`, and no test sends X-Real-Ip or X-Forwarded-For at all. Invariant 3 is entirely unverified by the suite; A3 is invisible.
- **Anything about the limiter's internals.** `ipLimiter.Allow` is never unit-tested directly: no window-expiry test, no Retry-After arithmetic test (the `kept[0].Add(window).Sub(now)` boundary and its negative clamp are untested), no concurrency test under `-race`, and nothing about map growth. The two limiter tests only prove 5-then-429 on a single key.
- **Concurrency of any kind.** No `-race` test, no parallel approve/hide, no parallel POSTs against the pending cap. A6 and A7 are untested.
- **Response header hygiene.** `TestSecurityHeadersPresent` checks four headers on `/healthz` only. Nothing asserts `Cache-Control` on the badge, its absence on `/admin` and `/e/{id}`, or the badge's narrowed CSP.
- **That the admin page does not leak.** No test asserts what an unauthenticated visitor sees; the leak tests all target `/e/`, `/api/entries` and `/badge`.
- **Log content.** Nothing asserts that bodies are not logged or that IPs are or are not.

## 4. Coverage
Read in full: cmd/itworks/main.go, internal/server/server.go, internal/server/handlers.go, internal/server/ratelimit.go, internal/server/view.go, internal/server/server_test.go, docker-compose.yml, .itworks/MAP.md, .itworks/LANES.md. Nothing in scope was skipped.
Read additionally for context (not lane A's to judge): internal/server/validate.go in full, web/templates/admin.html in full, and the function/SQL outline of internal/store/store.go (needed for A6, A7 and invariant 1).
Probes run: `go test ./internal/server` (pass), `go vet ./internal/server` (clean); a local instance on :18081 with trust_proxy off and one on :18082 with `ITWORKS_TRUST_PROXY=1`, both with `ITWORKS_ADMIN_USERS=matthew`, against which I ran curl matrices for admin path spellings (8 variants, `--path-as-is`), Origin values (13 variants), forwarded-username values (7 variants), submit-route path and content-type variants (9), header-rotation rate-limit runs under both trust settings, an oversized X-Real-Ip key, traversal attempts on `/e/`, `/badge/` and `/static/fonts/`, and response-header dumps for `/admin`, `/api/entries` and `/badge`. Both instances were stopped afterwards. No file in the repository was created, edited, or deleted; no agents were spawned.
