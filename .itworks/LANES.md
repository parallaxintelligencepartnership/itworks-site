# Review lanes: itworks

A lane is one reviewer's full read of a slice of the application cut along its identity, permission, stored-record and export boundaries. Lanes are re-derived when MAP.md's Layout table changes or a lane's listed files no longer exist. Re-derived 2026-09-16 after the move to a static GitHub Pages site. Two extra lanes (P and Q) cover the itworks plugin repo at ../itworks, because the site exists to receive that plugin's closeout entries and the two share one contract.

## Lane A: Build and publish
Lens id: security-auth
Files in scope:
- cmd/build/main.go, cmd/build/view.go (the renderer, output paths)
- .github/workflows/pages.yml (build and deploy)
- .github/workflows/check.yml (pull request gate and policy step)
Invariants to attack:
- A pull request from a non member cannot change anything outside entries/, cannot add more than one entry, cannot modify or delete an existing entry, and cannot make the deploy job run.
- The deploy job runs only from main and only with the pages permissions it needs; every action is pinned to a full commit sha.
- Output paths are derived only from validated ids, so no entry can write outside dist/ or over another entry's files.
- The masthead example badge is always captioned as an example and never taken from an unapproved entry.
Probe recipes:
- go test ./cmd/build -v; simulate diffs against the policy step as build_test.go does
Never re-flag:
- a pending entry has no badge until merge (DECISIONS 2026-09-16)
- merge is approval and delete is hide (DECISIONS 2026-09-16)

## Lane B: Records and rendering
Lens id: real-data
Files in scope:
- internal/entry/entry.go, load.go and their tests (the contract and the loader)
- internal/badge/badge.go and its test (SVG and colour rules)
- web/embed.go, web/templates/*.html, web/static/site.css
- fixtures/seed-msp-sentinel.json, docs/SPEC.md (the written contract)
Invariants to attack:
- Every submitter supplied string is escaped in HTML and cannot reach the SVG at all.
- Validation rejects every out-of-contract file: unknown fields, wrong types, bad dates, format and bidi characters, userinfo or non http(s) repo URLs, inconsistent counts, an id that does not match the filename rule.
- Badge colour is red when critical_open plus critical_accepted is above zero, else amber at 30 or more days, else green, UTC only.
- The wall lists every file under entries/ newest approved first and nothing else; approved_at comes from git and falls back to mtime with a warning.
Probe recipes:
- go test ./internal/entry ./internal/badge -v; go run ./cmd/build -check -entries <hostile dir>
Never re-flag:
- ISO dates are exempt from the no dash rule; badge plates darkened for AA and amber carries the word stale (DECISIONS 2026-09-11)
- accepted criticals count toward red and show separately (DECISIONS 2026-09-16)

## Lane P: Plugin lifecycle (../itworks)
Lens id: production-readiness
Files in scope: skills/*/SKILL.md, commands/*.md, hooks/*, .claude-plugin/*, README.md, CHANGELOG.md, ROADMAP.md
Invariants to attack:
- A novice following the skills cannot reach a public deploy without the secret scan, the auth check, the backup check, a runnable test suite and dependency vetting having actually run.
- Every path a skill names resolves from the plugin root; no skill contradicts another or the hook on the same event.
- Urgency, expertise claims and "not now" cannot disable closeout or audit; the hook cannot inject or leak project content.
- The published counts and badge never understate risk (an accepted CRITICAL is still a critical).
Probe recipes:
- bash -n hooks/session-start.sh and run it with and without .itworks; walk each skill as a novice
Never re-flag:
- profiles guided, brief, expert; closeout and audit pinned to Opus (2026-09-16)

## Lane Q: Plugin runbooks and contract (../itworks)
Lens id: security-auth
Files in scope: references/*.md, references/*.grep, scripts/itworks-lint.sh, tests/**, cross-repo ../itworks-site/internal/entry and docs/SPEC.md
Invariants to attack:
- Every runbook command runs on stock macOS and Ubuntu shells and catches the planted problem it claims to catch.
- publish.md's derivation rules produce a file internal/entry accepts.
- itworks-lint.sh never crashes on malformed state and never passes corruption.
Probe recipes:
- plant a fixture project and run every inspect step; run the lint on corrupted copies; validate publish.md's worked example with the site's entry package
Never re-flag:
- the audit tier is the only tier that reads application logic

## Not in scope
- web/static/fonts/* - binary font files
- docs/DESIGN-NOTES.md, docs/design/*, docs/audit/* - design record, mocks and retained audit evidence
- WORKFLOW-PROMPT.md - historical kickoff prompt
- .itworks/* - state files

## Lane-independent rules
- Reviewers read every file in scope in full and list any skip.
- Reviewers never edit the repository and never spawn agents.
- A suspicion becomes a finding only after a separate verifier reproduces it.
- Settled decisions go to "questions for the owner", never findings.
