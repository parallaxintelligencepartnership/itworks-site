# Lane A verification (Edge and identity, lens security-auth)

Repo: /Users/matthew/parallax-private/Projects/itworks-site @ 5a87acd. No repository file was created, edited or deleted. No ssh. No agents spawned.

Harness used for every app-side probe:

```
ITWORKS_DB=<scratch>/audit/work/verifyA.db ITWORKS_ADDR=:18091 \
ITWORKS_BASE_URL=http://localhost:18091 ITWORKS_ADMIN_USERS=matthew \
ITWORKS_TRUST_PROXY=1 go run ./cmd/itworks      # from the repo root, killed afterwards
```

Traefik harness (Docker was available; `docker info` succeeded): `traefik:v3` (digest
`sha256:f86a2cab1b5c649070c49f883c743dd32d8485a56e3368c5f93b9e91f1e91259`) in a container
built only to bake in a config file, listening on :18080, file provider with two routers
mirroring docker-compose.yml:

- `itworks` — `PathPrefix(`/`)`, priority 1, middleware `tag-public` (adds
  `X-Router-Tag: public-noauth`, adds **no** username header — mirrors the real public
  router, which also neutralises nothing).
- `itworks-admin` — `PathPrefix(`/admin`)`, priority 10, middleware `tag-auth`
  (`customRequestHeaders: X-Authentik-Username: matthew`) standing in for
  `authentik-auth@file`.
- Both point at `http://host.docker.internal:18091`, the running app. JSON access log to
  stdout, so `RouterName` is recorded per request. Container removed afterwards.

---

## A1. Percent-encoded /admin path bypassing the forward-auth router

**Verdict: PARTIAL.** The path-spelling bypass is **REFUTED**. The underlying
"the app has no independent proof that forward-auth ran" half is **CONFIRMED at IMPORTANT**,
which is exactly the downgrade the report itself asked for if Traefik decodes before matching.

### App-side half (reproduced, as claimed)

```
$ curl -s -o /dev/null -w '%{http_code}\n' --path-as-is -X POST \
    -H 'X-Authentik-Username: matthew' -H 'Origin: http://localhost:18091' \
    "http://localhost:18091/%61dmin/entries/abamwgyps6ck/approve"
303
$ curl -s localhost:18091/api/entries | grep -c abamwgyps6ck
1
```

Go's ServeMux (1.22+ pattern routing) matches on the **decoded** path, so `/%61dmin/...`
reaches `POST /admin/entries/{id}/approve`, and the only remaining check is the
client-suppliable `X-Authentik-Username`. That part of the report is accurate.

### Traefik half (the deciding fact) — decoded, not raw

Traefik v3 **decodes and cleans the path before matching**, and the decoded path is what it
logs and routes on. Two requests, neither carrying a client-supplied username header, both
landed on the admin router and were authenticated by the stand-in middleware:

```
POST /admin/entries/deofy45erzqy/approve   -> itworks-admin@file 303   (sent as /admin/...)
POST /admin/entries/4ls4pzry5gnt/approve   -> itworks-admin@file 303   (sent as /%61dmin/...)
```

Note the second line: the request was sent with `--path-as-is` as `/%61dmin/entries/...` and
Traefik recorded `RequestPath` as the **decoded** `/admin/entries/...` and matched
`itworks-admin@file`. Forward-auth applied. The claimed bypass does not exist on Traefik v3.

Full divergence hunt — every spelling the report raised plus more, each sent through Traefik
with a client-supplied `X-Authentik-Username: matthew` against a fresh pending entry:

| sent path | Traefik router | app result | approved? |
|---|---|---|---|
| `/%61dmin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/%2561dmin` | itworks@file | 404 | no |
| `//admin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/./admin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/e/../admin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/%2e/admin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/admin%2fentries` | itworks-admin@file | 404 | no |
| `/AdMin`, `/%41dmin` | itworks@file | 404 | no |
| `/admin/../admin` | itworks-admin@file | 303 | yes (auth'd router) |
| `/ad%6din` | itworks-admin@file | 303 | yes (auth'd router) |

**No spelling was found where Traefik chose the public router and Go chose an admin
handler.** Every spelling Go resolves to `/admin` Traefik also resolves to `/admin`; the two
spellings Traefik sent to the public router (`/%2561dmin`, `/%41dmin` -> `/Admin`) both 404 at
the app. Traefik is, if anything, *more* permissive than Go, which is the safe direction.

A second fact in the app's favour, observed by accident: the admin router's middleware
**overwrites** `X-Authentik-Username`, it does not append. A client-supplied
`X-Authentik-Username: nobody` sent through Traefik to `/%61dmin` came back 200 as `matthew`,
while the same request sent directly to the app logged `admin denied user="nobody"`. Real
Authentik forward-auth behaves the same way, so on the `/admin` prefix the client cannot
choose the identity.

### What is still real (the CONFIRMED residue)

- **docker-compose.yml does not strip `X-Authentik-Username` on the public router.** Read at
  5a87acd: `traefik.http.routers.itworks.middlewares=itworks-secheaders` only, and
  `itworks-secheaders` sets HSTS/frameDeny/nosniff/referrerPolicy — no
  `customRequestHeaders` clearing the identity header. So a client-supplied
  `X-Authentik-Username` survives on every non-`/admin` path. Today no admin-guarded route
  lives outside `/admin`, so this is latent, not live: it becomes live the day a guarded
  route is added outside the prefix, or the prefix rule is edited.
- **The app has no independent proof forward-auth ran.** `requireAdmin`
  (server.go:121-143) trusts a bare header. The container sits on the shared external
  network `dmz-infrastructure_dmz-public` with port 8080 open to every other container on it
  (A3 covers the same exposure). Anything with a foothold on that network reaches the app
  without passing Traefik at all and simply sets the header — full approve/hide as Matt.
  That is a real weakening gated on a second condition (a foothold on the shared network),
  which is IMPORTANT, not CRITICAL, under the rubric: a *stranger* on the internet cannot do it.

**Final severity: IMPORTANT.**

### Fix spec (A1)

What must change:

1. `internal/server/server.go`, `requireAdmin`: require a proof-of-forward-auth signal in
   addition to the username. Preferred shape: a shared secret header (e.g.
   `X-Itworks-Proxy-Auth`) compared with `crypto/subtle.ConstantTimeCompare` against a value
   read from an env var / Docker secret in `cmd/itworks/main.go`. If the env var is unset the
   server must refuse to start with admin routes enabled, rather than silently degrading.
   (Do NOT hardcode the secret in any file; per Matt's global rule it belongs in a Docker
   secret or `/etc/parallax/b2.env`.)
2. `internal/server/server.go`, `Routes`/`requireAdmin` (belt and braces, cheap): reject any
   request whose `r.URL.EscapedPath() != r.URL.Path` with 400 before the guard runs. This
   costs nothing, all legitimate ids are `[a-z0-9]`, and it makes the app safe regardless of
   what any future proxy's matcher does. It fails closed even if Traefik forwards the raw
   spelling.
3. `docker-compose.yml`, public router labels: add a middleware that clears the identity
   header on the public router, e.g.
   `traefik.http.middlewares.itworks-stripauth.headers.customRequestHeaders.X-Authentik-Username=""`
   and add `itworks-stripauth` to `traefik.http.routers.itworks.middlewares`. This is the
   Traefik-side half and is required even though the app-side fix lands.
4. `docker-compose.yml`, networking (answers the report's question 5): if nothing else on
   `dmz-infrastructure_dmz-public` needs to reach itworks directly, this is where the second
   condition gets removed.

Tests that must fail before and pass after (in `internal/server/server_test.go`):

- `TestAdminRequiresProxyAuthSignal` — `POST /admin/entries/<id>/approve` with
  `X-Authentik-Username: matthew`, a valid same-origin `Origin`, and **no** proxy-auth
  secret header. Expect 403. Today: 303.
- `TestAdminRejectsEncodedPath` — `POST /%61dmin/entries/<id>/approve` with a correct
  username and Origin. Expect 400 (or 404), and the entry must still be pending afterwards.
  Today: 303 and approved.

What must not change (settled, do not re-flag):

- Pending entries getting a badge stays as-is.
- `parseAdminUsers` behaviour: trims spaces, drops empties, empty allowlist denies
  everything, matching stays exact and case-sensitive. Header whitespace stripping by Go's
  header parser is correct.
- Router priorities (admin 10 vs public 1) and both routers staying `websecure` only.
- Forward-auth stays the primary guard; the app check stays defence in depth, not a
  replacement.

---

## A2. Same-origin check ignores the scheme and accepts a scheme-less or userinfo-bearing Origin

**Verdict: CONFIRMED. Severity: IMPORTANT.**

server.go:135-139 does `url.Parse(origin)` and compares `u.Host != r.Host` only. Scheme,
userinfo, path and case are all ignored on the Origin side, and `r.Host` is not normalised
either. Reproduced in full against `Host: localhost:18091`, `POST /admin/entries/{id}/approve`
with `X-Authentik-Username: matthew`:

```
  http://localhost:18091              303   <- accepted
  https://localhost:18091             303   <- accepted (scheme ignored)
  ftp://localhost:18091               303   <- accepted
  http://user:pw@localhost:18091      303   <- accepted (userinfo ignored)
  http://localhost:18091/evil         303   <- accepted (path ignored)
  //localhost:18091                   303   <- accepted (no scheme at all)
  http://evil.com                     403
  null                                403
  HTTP://LOCALHOST:18091              403
  http://localhost                    403
  (no Origin)                         403
```

Wrong outcome: any content that can be made to run at `http://itworks.dev`, or under any
other scheme handler resolving to that host:port, can forge a same-origin CSRF against
approve/hide while Matt's Authentik session is live. Needs a second condition (an active
network attacker before HSTS is pinned, or a stray plaintext vhost), hence IMPORTANT, not
CRITICAL. HSTS in docker-compose.yml mitigates but does not remove it. The
`HTTP://LOCALHOST:18091` denial is a bug in the other direction and confirms the comparison
is a raw string compare with no normalisation on either side.

### Fix spec (A2)

- `internal/server/server.go`, `requireAdmin`: build the expected origin from the request
  (`https://` + `r.Host` in production; derive the scheme from `ITWORKS_BASE_URL` so tests
  and local runs work over http) and compare the **whole** origin string after
  `strings.ToLower`, rejecting any Origin with a non-empty `u.User`, a non-empty `u.Path`, or
  an empty `u.Scheme`. Equivalent and simpler: compare against the configured
  `ITWORKS_BASE_URL` origin rather than against `r.Host`.
- Test `TestAdminOriginSchemeMismatch403` — table-driven over exactly the accepted list
  above: `https://localhost:PORT`, `ftp://localhost:PORT`,
  `http://user:pw@localhost:PORT`, `http://localhost:PORT/evil`, `//localhost:PORT`. Each
  must be 403 and leave the entry pending. Today all five are 303. Add
  `HTTP://LOCALHOST:PORT` as an *accept* case once normalisation lands.
- Must not change: a missing Origin on POST still denies; `null` still denies;
  `http://evil.example.com` still denies; GET `/admin` is still not Origin-checked.

---

## A3. X-Real-Ip trusted absolutely under ITWORKS_TRUST_PROXY=1

**Verdict: CONFIRMED. Severity: IMPORTANT.**

server.go:147-152 returns `r.Header.Get("X-Real-Ip")` verbatim whenever `trustProxy` is set,
with no trusted-proxy CIDR check on `r.RemoteAddr`. docker-compose.yml:9 sets
`ITWORKS_TRUST_PROXY: "1"` in production. Reproduced on the trust-proxy instance:

```
12 POSTs rotating X-Real-Ip 10.0.0.1..12 :  201 201 201 201 201 201 201 201 201 201 201 201
 7 POSTs with a fixed X-Real-Ip 10.0.5.5 :  201 201 201 201 201 429 429
```

The limiter keys entirely on an attacker-chosen string, so the quota is unlimited for anyone
who can set the header. The report's claim that `X-Forwarded-For` is never read also holds
(`X-Forwarded-For` appears nowhere in `internal/server`). Second condition: reaching the
container other than through Traefik, which the shared `dmz-infrastructure_dmz-public`
network (docker-compose.yml:13-14, 46-48, port 8080 unpublished but open on that network)
makes possible. Blast radius is the 500-entry pending cap (handlers.go:118-126), after which
every honest submitter gets 503.

### Fix spec (A3)

- `internal/server/server.go`, `clientIP`: only honour `X-Real-Ip` when the immediate peer
  (`r.RemoteAddr`) falls inside a configured trusted-proxy CIDR set; otherwise always use
  `RemoteAddr`. Add `ITWORKS_TRUSTED_PROXIES` (CIDR list) parsed in `cmd/itworks/main.go`
  alongside `parseAdminUsers`; `ITWORKS_TRUST_PROXY=1` with an empty CIDR list must fail to
  start rather than trust everyone. Also reject a header value that does not parse as a
  `net.IP` (see A4).
- Test `TestClientIPIgnoresXRealIPFromUntrustedPeer` — with `TrustProxy: true` and a trusted
  CIDR that excludes the test's loopback peer, 7 POSTs rotating
  `X-Real-Ip: 10.0.0.1..7` must produce 201x5 then 429. Today: seven 201s. Companion
  `TestClientIPHonoursXRealIPFromTrustedPeer` keeps the intended behaviour honest.
- Must not change: with trust_proxy unset the header stays ignored entirely (invariant 3's
  first half, which held); `X-Forwarded-For` stays unread; the IP stays out of the database.

---

## A4. Limiter map is never pruned; keys are arbitrary attacker-supplied strings

**Verdict: CONFIRMED. Severity: IMPORTANT.**

`internal/server/ratelimit.go:11-49`: every `Allow` writes `l.hits[ip] = kept`, including on
the deny path. `grep -c 'delete(' internal/server/ratelimit.go` returns **0**; there is no
janitor goroutine anywhere in the package. A key whose slice filters down to empty is still
written back as an empty slice and never removed. Under trust_proxy the key is whatever the
caller put in `X-Real-Ip`:

```
$ curl ... -H "X-Real-Ip: $(python3 -c "print('a'*3000)")" ... localhost:18091/api/entries
201
```

A 3000-character key was accepted and stored. Combined with A3, one caller mints unbounded
distinct map keys, each retaining the key string plus a `[]time.Time`, with no eviction until
process restart. Memory exhaustion takes down the wall, the badges and the admin page
together. Denial of service only, no disclosure, and gated on the same second condition as
A3 — IMPORTANT.

### Fix spec (A4)

- `internal/server/ratelimit.go`, `Allow`: when `len(kept) == 0` after the cutoff filter and
  the call is being denied or the slice would be stored empty, `delete(l.hits, ip)` instead of
  writing it back. Add a cheap opportunistic sweep: every N calls (or on a `time.Ticker`
  owned by the server), walk the map and delete keys whose newest hit is older than the
  window.
- `internal/server/server.go`, `clientIP`: parse the header with `net.ParseIP` and fall back
  to `RemoteAddr` when it does not parse, so the key space is bounded to real IPs. This is
  the same edit as A3's second bullet.
- Test `TestLimiterPrunesExpiredKeys` — construct `newIPLimiter(5, time.Minute)`, call
  `Allow("1.2.3.4", t0)`, then `Allow("5.6.7.8", t0.Add(2*time.Minute))`, and assert
  `len(l.hits) == 1`. Today: 2. Test `TestClientIPRejectsNonIPHeader` — `X-Real-Ip` of 3000
  `a`s must key on the RemoteAddr bucket, not its own.
- Must not change: 5-per-window semantics, the `Retry-After` arithmetic
  (`kept[0].Add(window).Sub(now)` with the negative clamp), the limiter being consumed before
  the body is touched (invariant 4, which held), and the limiter staying process-local.

---

## A5. Badge served with Cache-Control: public, max-age=3600

**Verdict: CONFIRMED. Severity: IMPORTANT.**

Reproduced end to end:

```
$ curl -s -D- -o /dev/null localhost:18091/badge/abamwgyps6ck.svg
HTTP/1.1 200 OK
Cache-Control: public, max-age=3600
$ curl -X POST ... /admin/entries/abamwgyps6ck/hide   -> 303
$ curl -s -o /dev/null -w '%{http_code}' localhost:18091/badge/abamwgyps6ck.svg
404
```

The origin does the right thing the moment Hide lands (404, handlers.go:157). The defect is
purely the hour of `public` caching it handed out beforehand (handlers.go:169).

**What GitHub's camo proxy does with Cache-Control.** Camo is a caching image proxy in front
of every external image in a README; it mirrors the origin's `Cache-Control` `max-age`
rather than imposing its own, so a badge served with `max-age=3600` is cached by camo for an
hour, on top of whatever the viewer's browser caches. The shields.io project hit exactly this
and worked around it by serving a short max-age: shields clamps any requested `cacheSeconds`
up to a **120 second floor** and serves `max-age=120` as its shortest badge lifetime, because
below that camo and the browser layers make the number meaningless anyway. There is no
public purge API for third parties: the only ways to evict a camo copy are to change the URL
or to use the GitHub-internal purge that the community `camo-purge` action drives with a
repo token. So the practical number for the fix is **120 seconds** — shields' own floor,
which is the shortest value that is honoured end to end.

Sources: [badges/shields#221 "The Cache-Control HTTP header is not enough to prevent GitHub's
CDN caching"](https://github.com/badges/shields/issues/221),
[badges/shields#726 "Cache-control header is 86400, despite setting maxAge"](https://github.com/badges/shields/issues/726),
[badges/shields#111](https://github.com/badges/shields/issues/111),
[kevincobain2000/action-camo-purge](https://github.com/kevincobain2000/action-camo-purge).

### Fix spec (A5)

- `internal/server/handlers.go`, `handleBadge` (line 169): change
  `Cache-Control: public, max-age=3600` to `public, max-age=120, must-revalidate` (120 is
  shields' floor, the shortest value third-party caches actually honour). Hide therefore
  takes effect for third parties within about two minutes plus one camo hop instead of an
  hour. Leave the 404-on-hidden path exactly as it is.
- Test `TestBadgeCacheControlIsShort` — `GET /badge/{id}.svg` on an approved entry must
  return a `Cache-Control` whose `max-age` is <= 120. Today it is 3600, so the test fails
  before and passes after. Pair it with `TestBadgeHiddenReturns404` if one does not already
  exist, so the shortened lifetime is not mistaken for the whole control.
- Must not change: pending entries still get a badge (settled, not re-flagged); the badge's
  narrowed CSP (`cspBadge`); the `/static/site.css` `max-age=86400`, which is a different
  asset with no takedown semantics.

---

## A6. Approve and hide have no state precondition

**Verdict: CONFIRMED. Severity: ADVISORY.**

```
approve 303
hide    303
approve 303      <- resurrected
on wall after resurrect: 1
approve again (already approved): 303
```

`internal/store/store.go:227-244`: `Approve` is an unconditional
`UPDATE entries SET status = ?, approved_at = ? WHERE id = ?` with no `WHERE status = ...`
guard, so a repeat approve re-stamps `approved_at`, which is the wall's sort key
(`ORDER BY approved_at DESC`), silently promoting an old entry to the top. `Hide` is
likewise unconditional and reversible. No transaction and no read-check-write, so a
concurrent approve and hide is last-writer-wins with no error to either caller. Both are
admin-only today and A1's stranger path is refuted, so this stays ADVISORY.

### Fix spec (A6)

- `internal/store/store.go`, `Approve`: add `AND status != 'approved'` (or take the current
  status as a parameter and enforce the allowed transition), and return a distinct
  `ErrInvalidTransition` when `RowsAffected` is 0 but the row exists, so `handleAdminApprove`
  can render a 409 rather than a silent 303.
- `internal/store/store.go`, `Hide`: decide first whether Hide is irreversible (the report's
  question 4 to the owner). If it is, `Approve` must also refuse a row whose status is
  `hidden`.
- Test `TestApproveTwiceDoesNotRestampApprovedAt` — approve, capture `approved_at`, approve
  again, assert `approved_at` is unchanged and the second call is not a plain 303. Today the
  second approve re-stamps.
- Must not change: `ErrNotFound` still maps to 404 in `handleAdminApprove`/`handleAdminHide`;
  the wall still sorts by `approved_at DESC`.

---

## A7. Pending cap is a read-then-write with no transaction

**Verdict: CONFIRMED (by trace). Severity: ADVISORY.**

Traced with exact values: `handlers.go:118` calls `s.db.CountPending()`
(`SELECT COUNT(*) FROM entries WHERE status = ?`, store.go:218-223) and gets, say, 499;
`handlers.go:122` compares `499 >= 500` -> false; `handlers.go:128` then calls
`s.db.Create(ne)` as a **separate** statement. Two goroutines interleaving between line 118
and line 128 both read 499 and both insert, giving 501. Nothing in the path opens a
transaction and there is no `UNIQUE`/`CHECK` backstop. SQLite serialising writers does not
help, because the overshoot is a read-before-commit, not a write race. Real impact is a soft
cap overshooting by roughly the number of concurrent POSTs, which the limiter bounds.

### Fix spec (A7)

- `internal/store/store.go`: add a `CreateIfUnderPendingCap(ne NewEntry, cap int) (string, error)`
  that wraps the count and the insert in a single `BEGIN IMMEDIATE` transaction and returns a
  sentinel `ErrPendingFull`. `internal/server/handlers.go:118-132` calls that instead of
  `CountPending` + `Create`, mapping `ErrPendingFull` to the existing 503 body.
- Test `TestPendingCapUnderConcurrency` — with `pendingCap` set to 5, fire 20 concurrent
  valid POSTs (limiter disabled or keyed per goroutine) and assert `CountPending() == 5`.
  Run under `-race`. Today it overshoots.
- Must not change: the 503 status and its existing message; the limiter still being consumed
  before the body is read.

---

## A8. Content-Type matched with HasPrefix

**Verdict: CONFIRMED. Severity: ADVISORY.**

```
$ curl -H 'Content-Type: application/jsonwhatever' -d '<valid payload>' localhost:18091/api/entries
201
```

`handlers.go:94-98` uses `strings.HasPrefix(ct, "application/json")`, so
`application/jsonwhatever` is accepted. Harmless today because `decodeSubmitRequest`
(validate.go:50-58) sets `DisallowUnknownFields` and rejects trailing data, but it is not the
exact check the invariant states.

### Fix spec (A8)

- `internal/server/handlers.go:94-98`: replace the prefix test with
  `mime.ParseMediaType(ct)` and require the media type to equal `application/json` exactly,
  tolerating a `charset=utf-8` parameter.
- Test `TestSubmitRejectsLookalikeContentType` — `Content-Type: application/jsonwhatever`
  must return 415; `application/json; charset=utf-8` must still return 201. Today the first
  returns 201.
- Must not change: the limiter is still consumed before the content-type check (invariant 4),
  so a 415 still costs the caller quota; the 413/400 split stays as it is (invariant 8).

---

## A9. No Cache-Control on private pages; usernames and IPs logged

**Verdict: CONFIRMED. Severity: ADVISORY.**

Header dump of `GET /admin` with a valid admin header returned 200 with CSP, nosniff,
Referrer-Policy and X-Frame-Options and **no** `Cache-Control` line. `GET /api/entries`
likewise: `grep -ic cache-control` over its response headers returned **0**. The admin page
lists every pending and hidden entry. Logging confirmed from the live log:

```
2026/09/16 18:14:43 admin action=approve id=abamwgyps6ck user=matthew result=ok
2026/09/16 18:14:43 admin action=hide    id=abamwgyps6ck user=matthew result=ok
```

plus `POST /api/entries ip=<client ip> status=<code>` from handlers.go:134,144. No payload
bodies are logged, and server.go:146 is correct that the IP is never persisted.

### Fix spec (A9)

- `internal/server/server.go`, `securityHeaders` (or per-handler in `handleAdminList` and
  `handleEntry`): set `Cache-Control: no-store` on `/admin`, `/e/{id}` and `/api/entries`.
  Cleanest shape: default every response to `no-store` in `securityHeaders` and let the two
  handlers that want caching (`handleBadge`, `handleStaticCSS`, `handleStaticFont`) overwrite
  it, since they already set their own.
- `internal/server/handlers.go:134,144`: reconcile the `ip=` field with lane C's invariant
  and PROJECT.md. If visitor IPs are not permitted in logs, log a truncated or hashed form.
  The admin username at handlers.go:282 is an audit record and should stay.
- Test `TestPrivatePagesAreNoStore` — `GET /admin` (authenticated), `GET /e/{id}` and
  `GET /api/entries` must each carry `Cache-Control: no-store`. Today none of the three does.
- Must not change: the badge's own `Cache-Control` (A5 sets that number) and the CSS/font
  `max-age=86400`; the four existing security headers on every response.

---

## Invariants and never-re-flag list carried forward

Re-confirmed during this verification and **not** to be re-opened: pending entries get a
badge (settled); `parseAdminUsers` trimming/empty-drop/deny-on-empty and exact
case-sensitive matching; the limiter being consumed before the body is touched; the
admin router priority 10 vs public 1 and both routers being `websecure` only; forward-auth
as the primary guard with the app check as defence in depth; `X-Forwarded-For` staying
unread; the client IP never reaching the database.

Newly settled by this verification and not to be re-flagged: **Traefik v3 decodes and
normalises the path before rule matching**, so `PathPrefix(`/admin`)` covers `/%61dmin`,
`//admin`, `/./admin`, `/e/../admin`, `/%2e/admin`, `/admin/../admin` and `/ad%6din`; and the
Authentik middleware **overwrites** rather than appends `X-Authentik-Username`, so identity
cannot be chosen by the client on any path the admin router serves.

---

## Summary

| Suspicion | Verdict | Final severity |
|---|---|---|
| A1 percent-encoded /admin bypasses forward-auth | PARTIAL (path bypass REFUTED; unverified forward-auth trust CONFIRMED) | IMPORTANT |
| A2 same-origin check ignores scheme/userinfo | CONFIRMED | IMPORTANT |
| A3 X-Real-Ip trusted absolutely under trust_proxy | CONFIRMED | IMPORTANT |
| A4 limiter map never pruned, unbounded keys | CONFIRMED | IMPORTANT |
| A5 badge Cache-Control: public, max-age=3600 | CONFIRMED | IMPORTANT |
| A6 approve/hide have no state precondition | CONFIRMED | ADVISORY |
| A7 pending cap is read-then-write, no transaction | CONFIRMED (traced) | ADVISORY |
| A8 Content-Type matched with HasPrefix | CONFIRMED | ADVISORY |
| A9 no Cache-Control on private pages; IPs/usernames logged | CONFIRMED | ADVISORY |
