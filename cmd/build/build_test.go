package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"itworks.build/internal/entry"
)

// fixtureID is a well formed entry id: 12 characters of lowercase base32.
const fixtureID = "mspsentinel2"

var buildToday = "2026-09-16"

// entriesDirWithFixture copies the repo fixture into a fresh directory
// under the id it would carry once merged.
func entriesDirWithFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "seed-msp-sentinel.json"))
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fixtureID+".json"), body, 0o644); err != nil {
		t.Fatalf("write the entry: %v", err)
	}
	return dir
}

func mustRun(t *testing.T, entriesDir, outDir, today string, check bool) string {
	t.Helper()
	var warn bytes.Buffer
	if err := run(entriesDir, outDir, today, check, &warn); err != nil {
		t.Fatalf("run: %v\n%s", err, warn.String())
	}
	return warn.String()
}

func readFile(t *testing.T, parts ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatalf("read %v: %v", parts, err)
	}
	return string(b)
}

func TestBuildRendersEverySiteFile(t *testing.T) {
	out := t.TempDir()
	mustRun(t, entriesDirWithFixture(t), out, buildToday, false)

	want := []string{
		"index.html",
		filepath.Join("wall", "index.html"),
		filepath.Join("e", fixtureID, "index.html"),
		filepath.Join("badge", fixtureID+".svg"),
		filepath.Join("api", "entries.json"),
		"404.html",
		filepath.Join("static", "site.css"),
		filepath.Join("static", "embed.js"),
		filepath.Join("static", "favicon.svg"),
		filepath.Join("static", "favicon-32.png"),
		filepath.Join("static", "apple-touch-icon.png"),
		"CNAME",
		".nojekyll",
	}
	for _, rel := range want {
		if _, err := os.Stat(filepath.Join(out, rel)); err != nil {
			t.Fatalf("missing output %s: %v", rel, err)
		}
	}

	if got := readFile(t, out, "CNAME"); strings.TrimSpace(got) != "itworks.build" {
		t.Fatalf("CNAME = %q, want itworks.build", got)
	}

	// Every page carries the icon link, so a tab is never blank.
	for _, page := range []string{"index.html", filepath.Join("wall", "index.html"), "404.html"} {
		if !strings.Contains(readFile(t, out, page), `<link rel="icon" href="/static/favicon.svg"`) {
			t.Fatalf("%s is missing the favicon link", page)
		}
	}

	// The badge has to be well formed XML, since every other site on the
	// web renders it as an image.
	svg := readFile(t, out, "badge", fixtureID+".svg")
	dec := xml.NewDecoder(strings.NewReader(svg))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("badge SVG does not parse: %v", err)
		}
	}

	// The wall lists the entry and links to its page and its badge.
	wall := readFile(t, out, "wall", "index.html")
	for _, want := range []string{"MSP Sentinel", "/e/" + fixtureID + "/", "/badge/" + fixtureID + ".svg"} {
		if !strings.Contains(wall, want) {
			t.Fatalf("wall page is missing %q", want)
		}
	}

	// The entry page keeps the two critical counts apart.
	page := readFile(t, out, "e", fixtureID, "index.html")
	for _, want := range []string{"critical open", "critical accepted by the owner"} {
		if !strings.Contains(page, want) {
			t.Fatalf("entry page is missing %q", want)
		}
	}
}

// TestBuildRemovesEntriesDeletedFromTheSource is the ADVISORY fix: render
// never cleared outDir, so a removed entry's page and badge survived an
// incremental local build.
func TestBuildRemovesEntriesDeletedFromTheSource(t *testing.T) {
	entriesDir := entriesDirWithFixture(t)
	out := t.TempDir()
	mustRun(t, entriesDir, out, buildToday, false)

	page := filepath.Join(out, "e", fixtureID, "index.html")
	badge := filepath.Join(out, "badge", fixtureID+".svg")
	if _, err := os.Stat(page); err != nil {
		t.Fatalf("first build missing entry page: %v", err)
	}
	if _, err := os.Stat(badge); err != nil {
		t.Fatalf("first build missing badge: %v", err)
	}

	if err := os.Remove(filepath.Join(entriesDir, fixtureID+".json")); err != nil {
		t.Fatalf("remove the entry: %v", err)
	}
	mustRun(t, entriesDir, out, buildToday, false)

	if _, err := os.Stat(page); !os.IsNotExist(err) {
		t.Fatalf("entry page still exists after the entry was removed: err=%v", err)
	}
	if _, err := os.Stat(badge); !os.IsNotExist(err) {
		t.Fatalf("badge still exists after the entry was removed: err=%v", err)
	}
	// The rest of the site is still there.
	if _, err := os.Stat(filepath.Join(out, "index.html")); err != nil {
		t.Fatalf("index.html missing after rebuild: %v", err)
	}

	wall := readFile(t, out, "wall", "index.html")
	if strings.Contains(wall, "MSP Sentinel") {
		t.Fatalf("wall still names the removed entry:\n%s", wall)
	}
	if strings.Contains(wall, "/e/"+fixtureID+"/") {
		t.Fatalf("wall still links the removed entry's page:\n%s", wall)
	}

	sitemap := readFile(t, out, "sitemap.xml")
	if strings.Contains(sitemap, "/e/"+fixtureID+"/") {
		t.Fatalf("sitemap still lists the removed entry's page:\n%s", sitemap)
	}
}

func TestBuildFeedRoundTripsThroughValidation(t *testing.T) {
	out := t.TempDir()
	mustRun(t, entriesDirWithFixture(t), out, buildToday, false)

	var feed struct {
		BuiltAt string            `json:"built_at"`
		Entries []json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal([]byte(readFile(t, out, "api", "entries.json")), &feed); err != nil {
		t.Fatalf("entries.json does not parse: %v", err)
	}
	if len(feed.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(feed.Entries))
	}
	builtAt, err := time.Parse(time.RFC3339, feed.BuiltAt)
	if err != nil {
		t.Fatalf("built_at = %q does not parse as RFC 3339: %v", feed.BuiltAt, err)
	}
	if d := time.Since(builtAt); d < 0 || d > 5*time.Minute {
		t.Fatalf("built_at = %s is not within 5 minutes of now", feed.BuiltAt)
	}

	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	for _, raw := range feed.Entries {
		var rec entry.Record
		if err := json.Unmarshal(raw, &rec); err != nil {
			t.Fatalf("entry does not decode into a record: %v", err)
		}
		if _, msg := entry.Validate(rec, now); msg != "" {
			t.Fatalf("published entry fails validation: %s", msg)
		}
		var shape map[string]any
		if err := json.Unmarshal(raw, &shape); err != nil {
			t.Fatalf("entry does not decode: %v", err)
		}
		for _, key := range []string{"id", "name", "summary", "source", "repo_url", "audit_tier",
			"audit_date", "found", "fixed", "accepted", "critical_open", "critical_accepted",
			"age_days", "badge_color", "badge_url", "entry_url"} {
			if _, ok := shape[key]; !ok {
				t.Fatalf("entries.json is missing the %s field", key)
			}
		}
		if len(shape) != 16 {
			t.Fatalf("entries.json carries %d fields, want the 16 of the published shape", len(shape))
		}
	}
}

func TestBuildWithNoEntriesShowsTheExampleBadge(t *testing.T) {
	out := t.TempDir()
	mustRun(t, t.TempDir(), out, buildToday, false)

	index := readFile(t, out, "index.html")
	if !strings.Contains(index, `src="/badge/example-green.svg"`) {
		t.Fatalf("landing page does not show the example badge:\n%s", index)
	}
	if !strings.Contains(index, "example") {
		t.Fatalf("landing page does not caption the example badge")
	}
	if !strings.Contains(index, "Nothing on the wall yet") {
		t.Fatalf("landing page does not say the wall is empty")
	}
	for _, name := range []string{"example-green.svg", "example-amber.svg", "example-red.svg"} {
		if _, err := os.Stat(filepath.Join(out, "badge", name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

// TestRenderEscapesHostileName is the B4 fix: a name that is itself markup
// is legal input (it is printable text under the 60 character limit), and
// html/template must escape it everywhere it lands in a page, while the
// JSON feed must round trip it unchanged.
func TestRenderEscapesHostileName(t *testing.T) {
	const hostileID = "hostilenameq"
	const hostileName = "<script>alert(1)</script>"

	rec := map[string]any{
		"name":              hostileName,
		"summary":           "a name that is itself markup",
		"source":            "closed",
		"repo_url":          "",
		"audit_tier":        "audit",
		"audit_date":        "2026-09-07",
		"found":             1,
		"fixed":             1,
		"accepted":          0,
		"critical_open":     0,
		"critical_accepted": 0,
	}
	body, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, hostileID+".json"), body, 0o644); err != nil {
		t.Fatalf("write the entry: %v", err)
	}

	out := t.TempDir()
	mustRun(t, dir, out, buildToday, false)

	for _, page := range [][]string{
		{out, "index.html"},
		{out, "wall", "index.html"},
		{out, "e", hostileID, "index.html"},
	} {
		html := readFile(t, page...)
		if !strings.Contains(html, "&lt;script&gt;") {
			t.Fatalf("%v does not escape the hostile name", page)
		}
		if strings.Contains(html, "<script>alert") {
			t.Fatalf("%v renders the hostile name unescaped", page)
		}
	}

	feed := readFile(t, out, "api", "entries.json")
	if !strings.Contains(feed, "<script>") && !strings.Contains(feed, "\\u003cscript\\u003e") {
		t.Fatalf("entries.json neither carries a raw <script> nor the encoding/json escaped form:\n%s", feed)
	}
	var decoded struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(feed), &decoded); err != nil {
		t.Fatalf("entries.json does not parse: %v", err)
	}
	if len(decoded.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(decoded.Entries))
	}
	if decoded.Entries[0].Name != hostileName {
		t.Fatalf("name = %q, want %q (round trip through JSON must not alter it)", decoded.Entries[0].Name, hostileName)
	}
}

// TestCheckFailsOnAHostileEntry covers the B1 and B2 inputs reaching the
// build: a name carrying a right to left override and a repo url whose
// userinfo makes it read as another host.
func TestCheckFailsOnAHostileEntry(t *testing.T) {
	hostile := `{
  "name": "paypal` + string(rune(0x202E)) + ` gro.live",
  "summary": "bidi and zero width probe",
  "source": "public",
  "repo_url": "https://github.com@evil.example.com/x",
  "audit_tier": "audit",
  "audit_date": "2026-09-16",
  "found": 5,
  "fixed": 1,
  "accepted": 1,
  "critical_open": 0,
  "critical_accepted": 0
}`
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hostileentry.json"), []byte(hostile), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	out := filepath.Join(t.TempDir(), "dist")
	var warn bytes.Buffer
	if err := run(dir, out, buildToday, true, &warn); err == nil {
		t.Fatalf("check passed a hostile entry")
	}
	if !strings.Contains(warn.String(), "hostileentry.json") {
		t.Fatalf("check did not name the offending file: %q", warn.String())
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("check wrote %s; it must write nothing", out)
	}
}

func TestCheckPassesAnEmptyEntriesDirectory(t *testing.T) {
	out := filepath.Join(t.TempDir(), "dist")
	mustRun(t, t.TempDir(), out, buildToday, true)
	if _, err := os.Stat(out); err == nil {
		t.Fatalf("check wrote %s; it must write nothing", out)
	}
}

// TestWallOrderIsNewestFirstThenByID pins the order two entries appear in
// when they were approved in the same second.
func TestWallOrderIsNewestFirstThenByID(t *testing.T) {
	older := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	same := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	entries := []entry.Entry{
		{ID: "bbbbbbbbbbbb", Name: "B", AuditDate: "2026-09-01", ApprovedAt: same},
		{ID: "cccccccccccc", Name: "C", AuditDate: "2026-09-01", ApprovedAt: older},
		{ID: "aaaaaaaaaaaa", Name: "A", AuditDate: "2026-09-01", ApprovedAt: same},
	}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	wall := readFile(t, out, "wall", "index.html")
	want := []string{"aaaaaaaaaaaa", "bbbbbbbbbbbb", "cccccccccccc"}
	at := -1
	for _, id := range want {
		i := strings.Index(wall, "/e/"+id+"/")
		if i < 0 {
			t.Fatalf("wall is missing %s", id)
		}
		if i < at {
			t.Fatalf("wall order is wrong; %s came too late", id)
		}
		at = i
	}
}

// TestRepoLinksCarryRel is the B3 fix: a link a submitter chose must not
// pass the site's standing on to whatever it points at.
func TestRepoLinksCarryRel(t *testing.T) {
	entries := []entry.Entry{{
		ID: "publicrepo23", Name: "Public Repo", Summary: "has a repo link",
		Source: "public", RepoURL: "https://example.com/a", AuditTier: "audit",
		AuditDate: "2026-09-01", Found: 1, Fixed: 1,
		ApprovedAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	for _, page := range [][]string{
		{out, "wall", "index.html"},
		{out, "e", "publicrepo23", "index.html"},
	} {
		body := readFile(t, page...)
		i := strings.Index(body, `href="https://example.com/a"`)
		if i < 0 {
			t.Fatalf("%v does not link the repo", page)
		}
		tag := body[i : i+120]
		if !strings.Contains(tag, `rel="nofollow noopener noreferrer"`) {
			t.Fatalf("%v repo link has no rel: %s", page, tag)
		}
	}
}

// TestEntryDescriptionNamesAcceptedCriticals covers the description a
// crawler and a link preview get: when criticals are accepted, not just
// open, the description says both numbers rather than only one.
func TestEntryDescriptionNamesAcceptedCriticals(t *testing.T) {
	entries := []entry.Entry{{
		ID: "acceptedonly1", Name: "Accepted Only", Summary: "no criticals open, two accepted",
		Source: "closed", AuditTier: "audit", AuditDate: "2026-09-16",
		Found: 3, Fixed: 1, Accepted: 2, CriticalOpen: 0, CriticalAccepted: 2,
		ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := readFile(t, out, "e", "acceptedonly1", "index.html")
	if !strings.Contains(page, `<meta name="description" content="`) {
		t.Fatalf("entry page has no description meta")
	}
	i := strings.Index(page, `<meta name="description" content="`)
	rest := page[i+len(`<meta name="description" content="`):]
	desc := rest[:strings.Index(rest, `"`)]
	if !strings.Contains(desc, "2 critical accepted") {
		t.Fatalf("entry description does not mention accepted criticals: %q", desc)
	}

	j := strings.Index(page, `<meta property="og:description" content="`)
	if j < 0 {
		t.Fatalf("entry page has no og:description")
	}
	restJ := page[j+len(`<meta property="og:description" content="`):]
	ogDesc := restJ[:strings.Index(restJ, `"`)]
	if !strings.Contains(ogDesc, "2 critical accepted") {
		t.Fatalf("entry og:description does not mention accepted criticals: %q", ogDesc)
	}
}

// TestEntryPageFallsBackWhenRepoURLIsEmpty covers the notice for an entry
// that never gave a repo link: the page says so instead of leaving a blank.
func TestEntryPageFallsBackWhenRepoURLIsEmpty(t *testing.T) {
	entries := []entry.Entry{{
		ID: "norepourl001", Name: "No Repo URL", Summary: "never gave a repo link",
		Source: "private", AuditTier: "audit", AuditDate: "2026-09-16",
		Found: 1, Fixed: 1, ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := readFile(t, out, "e", "norepourl001", "index.html")
	if !strings.Contains(page, "source not published") {
		t.Fatalf("entry page does not fall back to \"source not published\":\n%s", page)
	}
}

// TestEntryPageCarriesTheBadgeMarkdownSnippet covers the "post it
// yourself" block: it hands the entry's own badge URL in the markdown a
// README embeds.
func TestEntryPageCarriesTheBadgeMarkdownSnippet(t *testing.T) {
	entries := []entry.Entry{{
		ID: "badgemdsnip1", Name: "Badge Markdown", Summary: "carries its own badge URL",
		Source: "public", AuditTier: "audit", AuditDate: "2026-09-16",
		Found: 1, Fixed: 1, ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	page := readFile(t, out, "e", "badgemdsnip1", "index.html")
	want := "![audit badge](https://itworks.build/badge/badgemdsnip1.svg)"
	if !strings.Contains(page, want) {
		t.Fatalf("entry page is missing the badge markdown snippet %q:\n%s", want, page)
	}
}

// TestEntryPageStateWordsAndClasses covers the state a reader gets on the
// entry page itself: a critical open reads "critical open" in the f-crit
// flap, and an audit past 30 days reads "stale" in the f-stale flap, with
// no crossover between the two.
func TestEntryPageStateWordsAndClasses(t *testing.T) {
	today := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)
	entries := []entry.Entry{
		{
			ID: "criticalopen1", Name: "Critical Open Entry", Summary: "one critical still open",
			Source: "public", AuditTier: "audit", AuditDate: "2026-09-16",
			Found: 1, Fixed: 0, CriticalOpen: 1, ApprovedAt: today,
		},
		{
			ID: "staleentry01", Name: "Stale Entry", Summary: "audited 40 days back",
			Source: "public", AuditTier: "audit", AuditDate: "2026-08-07",
			Found: 1, Fixed: 1, ApprovedAt: today,
		},
	}
	out := t.TempDir()
	if err := render(entries, out, today); err != nil {
		t.Fatalf("render: %v", err)
	}

	critPage := readFile(t, out, "e", "criticalopen1", "index.html")
	if !strings.Contains(critPage, "reads critical open") {
		t.Fatalf("critical entry page does not read \"critical open\":\n%s", critPage)
	}
	if !strings.Contains(critPage, `class="flap f-crit"`) {
		t.Fatalf("critical entry page does not carry the f-crit flap class:\n%s", critPage)
	}

	stalePage := readFile(t, out, "e", "staleentry01", "index.html")
	if !strings.Contains(stalePage, "reads stale") {
		t.Fatalf("stale entry page does not read \"stale\":\n%s", stalePage)
	}
	if !strings.Contains(stalePage, `class="flap f-stale"`) {
		t.Fatalf("stale entry page does not carry the f-stale flap class:\n%s", stalePage)
	}
}

func TestBadgeColorCountsAcceptedCriticals(t *testing.T) {
	entries := []entry.Entry{{
		ID: "acceptedcrit", Name: "Accepted Crit", Summary: "one critical accepted, none open",
		Source: "closed", AuditTier: "audit", AuditDate: "2026-09-16",
		Found: 3, Fixed: 2, Accepted: 1, CriticalOpen: 0, CriticalAccepted: 1,
		ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	svg := readFile(t, out, "badge", "acceptedcrit.svg")
	if !strings.Contains(svg, "#b3261e") {
		t.Fatalf("badge is not red for an accepted critical:\n%s", svg)
	}
	if !strings.Contains(svg, "1 critical") {
		t.Fatalf("badge does not count the accepted critical:\n%s", svg)
	}
	// The entry page keeps the numbers apart, never added into one word.
	page := readFile(t, out, "e", "acceptedcrit", "index.html")
	if !strings.Contains(page, `<span class="k">critical open</span><span class="v num">0</span>`) {
		t.Fatalf("entry page does not show 0 critical open:\n%s", page)
	}
	if !strings.Contains(page, `<span class="k">critical accepted by the owner</span><span class="v num">1</span>`) {
		t.Fatalf("entry page does not show 1 critical accepted:\n%s", page)
	}
}

func TestWallCriticalCellCountsAcceptedCriticals(t *testing.T) {
	entries := []entry.Entry{
		{
			ID: "acceptedcrit", Name: "Accepted Crit", Summary: "one critical accepted, none open",
			Source: "closed", AuditTier: "audit", AuditDate: "2026-09-16",
			Found: 3, Fixed: 2, Accepted: 1, CriticalOpen: 0, CriticalAccepted: 1,
			ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			ID: "cleanentry01", Name: "Clean Entry", Summary: "nothing critical at all",
			Source: "closed", AuditTier: "audit", AuditDate: "2026-09-16",
			Found: 1, Fixed: 1, Accepted: 0, CriticalOpen: 0, CriticalAccepted: 0,
			ApprovedAt: time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC),
		},
	}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	wall := readFile(t, out, "wall", "index.html")

	i := strings.Index(wall, `id="acceptedcrit"`)
	if i < 0 {
		i = strings.Index(wall, "/e/acceptedcrit/")
	}
	if i < 0 {
		t.Fatalf("wall is missing acceptedcrit")
	}
	end := strings.Index(wall[i:], `<a class="plate-frame"`)
	if end < 0 {
		t.Fatalf("wall row for acceptedcrit has no badge plate")
	}
	row := wall[i : i+end]
	if !strings.Contains(row, `<span class="k">critical</span><span class="v num">1</span><span class="note">1 accepted</span>`) {
		t.Fatalf("wall critical cell does not count the accepted critical:\n%s", row)
	}
	if strings.Contains(row, "c-zero") {
		t.Fatalf("wall critical cell styles an accepted critical as zero:\n%s", row)
	}

	j := strings.Index(wall, "/e/cleanentry01/")
	if j < 0 {
		t.Fatalf("wall is missing cleanentry01")
	}
	endJ := strings.Index(wall[j:], `<a class="plate-frame"`)
	if endJ < 0 {
		t.Fatalf("wall row for cleanentry01 has no badge plate")
	}
	rowJ := wall[j : j+endJ]
	if !strings.Contains(rowJ, `<span class="k">critical</span><span class="v num">0</span></div>`) {
		t.Fatalf("wall critical cell does not show 0 with nothing appended:\n%s", rowJ)
	}
	if strings.Contains(rowJ, `class="note"`) {
		t.Fatalf("wall critical cell should not mention accepted for a clean entry:\n%s", rowJ)
	}
	if !strings.Contains(rowJ, "c-zero") {
		t.Fatalf("wall critical cell does not style a clean entry as zero:\n%s", rowJ)
	}
}

// pageFiles are the four page kinds, keyed by the path they are written
// to, for the tests that hold for every page on the site.
func pageFiles(out string) map[string]string {
	return map[string]string{
		"index.html": "index.html",
		"wall":       filepath.Join("wall", "index.html"),
		"entry":      filepath.Join("e", fixtureID, "index.html"),
		"404.html":   "404.html",
	}
}

// TestEveryPageIsCrawlable covers the head a search engine reads: the page
// has to name its own absolute address, describe itself in its own words,
// and carry the publisher graph.
func TestEveryPageIsCrawlable(t *testing.T) {
	out := t.TempDir()
	mustRun(t, entriesDirWithFixture(t), out, buildToday, false)

	canonicals := map[string]string{
		"index.html": "https://itworks.build/",
		"wall":       "https://itworks.build/wall/",
		"entry":      "https://itworks.build/e/" + fixtureID + "/",
		"404.html":   "https://itworks.build/404.html",
	}
	seen := map[string]string{}
	for name, rel := range pageFiles(out) {
		body := readFile(t, out, rel)

		want := `<link rel="canonical" href="` + canonicals[name] + `">`
		if !strings.Contains(body, want) {
			t.Fatalf("%s is missing %s", name, want)
		}
		if !strings.Contains(body, `<meta property="og:url" content="`+canonicals[name]+`">`) {
			t.Fatalf("%s og:url is not its canonical URL", name)
		}
		for _, want := range []string{
			`<meta property="og:type" content="website">`,
			`<meta property="og:site_name" content="itworks.build">`,
			`<meta name="twitter:card" content="summary">`,
			`<meta property="og:title" content=`,
			`<meta property="og:description" content=`,
		} {
			if !strings.Contains(body, want) {
				t.Fatalf("%s is missing %s", name, want)
			}
		}

		// The description is the page's own, not a site wide boilerplate.
		i := strings.Index(body, `<meta name="description" content="`)
		if i < 0 {
			t.Fatalf("%s has no description", name)
		}
		rest := body[i+len(`<meta name="description" content="`):]
		desc := rest[:strings.Index(rest, `"`)]
		if len(desc) < 40 {
			t.Fatalf("%s description is too thin: %q", name, desc)
		}
		if other, dup := seen[desc]; dup {
			t.Fatalf("%s repeats the description of %s", name, other)
		}
		seen[desc] = name

		// One JSON-LD block, naming the publisher and the site, and it has
		// to parse as JSON.
		const open = `<script type="application/ld+json">`
		j := strings.Index(body, open)
		if j < 0 {
			t.Fatalf("%s has no JSON-LD", name)
		}
		raw := body[j+len(open):]
		raw = raw[:strings.Index(raw, `</script>`)]
		var doc struct {
			Context string `json:"@context"`
			Graph   []struct {
				Type   string   `json:"@type"`
				Name   string   `json:"name"`
				URL    string   `json:"url"`
				Email  string   `json:"email"`
				SameAs []string `json:"sameAs"`
				Addr   struct {
					Locality string `json:"addressLocality"`
					Region   string `json:"addressRegion"`
					Country  string `json:"addressCountry"`
				} `json:"address"`
			} `json:"@graph"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("%s JSON-LD does not parse: %v", name, err)
		}
		if doc.Context != "https://schema.org" {
			t.Fatalf("%s JSON-LD @context = %q", name, doc.Context)
		}
		if len(doc.Graph) != 2 {
			t.Fatalf("%s JSON-LD has %d nodes, want the organization and the website", name, len(doc.Graph))
		}
		org, site := doc.Graph[0], doc.Graph[1]
		if org.Type != "Organization" || org.Name != "Parallax Intelligence Partnership, LLC" {
			t.Fatalf("%s JSON-LD organization = %+v", name, org)
		}
		if org.URL != "https://parallaxintelligence.ai" || org.Email != "hello@parallaxintelligence.ai" {
			t.Fatalf("%s JSON-LD organization url or email is wrong: %+v", name, org)
		}
		if org.Addr.Locality != "Battle Creek" || org.Addr.Region != "MI" || org.Addr.Country != "US" {
			t.Fatalf("%s JSON-LD address = %+v", name, org.Addr)
		}
		for _, want := range []string{
			"https://parallaxintelligence.digital",
			"https://stillpub.app",
			"https://postmortem.report",
			"https://github.com/parallaxintelligencepartnership",
		} {
			if !slicesContains(org.SameAs, want) {
				t.Fatalf("%s JSON-LD sameAs is missing %s", name, want)
			}
		}
		if site.Type != "WebSite" || site.Name != "itworks.build" || site.URL != "https://itworks.build" {
			t.Fatalf("%s JSON-LD website = %+v", name, site)
		}
	}
}

func slicesContains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestSitemapAndRobots covers what a crawler is handed at the root: every
// page of the site with the day it last changed, and a robots.txt that
// invites the crawl and points at the sitemap.
func TestSitemapAndRobots(t *testing.T) {
	approved := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	entries := []entry.Entry{{
		ID: "sitemapentr1", Name: "Sitemap Entry", Summary: "in the sitemap",
		Source: "public", AuditTier: "audit", AuditDate: "2026-09-10",
		Found: 2, Fixed: 2, ApprovedAt: approved,
	}}
	out := t.TempDir()
	if err := render(entries, out, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("render: %v", err)
	}

	var doc struct {
		XMLName xml.Name `xml:"urlset"`
		URLs    []struct {
			Loc     string `xml:"loc"`
			LastMod string `xml:"lastmod"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(readFile(t, out, "sitemap.xml")), &doc); err != nil {
		t.Fatalf("sitemap.xml does not parse: %v", err)
	}
	got := map[string]string{}
	for _, u := range doc.URLs {
		got[u.Loc] = u.LastMod
	}
	for _, want := range []string{"https://itworks.build/", "https://itworks.build/wall/"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("sitemap does not list %s: %+v", want, got)
		}
	}
	const entryURL = "https://itworks.build/e/sitemapentr1/"
	if got[entryURL] != "2026-09-12" {
		t.Fatalf("sitemap lastmod for the entry = %q, want its approval date 2026-09-12", got[entryURL])
	}

	robots := readFile(t, out, "robots.txt")
	for _, want := range []string{"User-agent: *", "Allow: /", "Sitemap: https://itworks.build/sitemap.xml"} {
		if !strings.Contains(robots, want) {
			t.Fatalf("robots.txt is missing %q:\n%s", want, robots)
		}
	}
}

// TestFooterCarriesTheRecordAndFollowsItsLinks pins the footer rules: the
// legal name is stated once, the sister sites and the repos are listed
// under headings that say what they are, every outbound link is followed,
// and LinkedIn is nowhere.
func TestFooterCarriesTheRecordAndFollowsItsLinks(t *testing.T) {
	out := t.TempDir()
	mustRun(t, entriesDirWithFixture(t), out, buildToday, false)

	for name, rel := range pageFiles(out) {
		body := readFile(t, out, rel)
		i := strings.Index(body, "<footer")
		if i < 0 {
			t.Fatalf("%s has no footer", name)
		}
		foot := body[i:]

		// Stated once, as the identity block. The JSON-LD says it again
		// for a machine, which is not copy on the page.
		if n := strings.Count(foot, "Parallax Intelligence Partnership, LLC"); n != 1 {
			t.Fatalf("%s footer names the full legal entity %d times, want exactly once", name, n)
		}
		for _, want := range []string{
			"Battle Creek, Michigan",
			`href="mailto:hello@parallaxintelligence.ai"`,
			"This site never runs a model.",
			"&copy; 2026",
			// sister sites
			`href="https://parallaxintelligence.ai"`,
			`href="https://parallaxintelligence.digital"`,
			`href="https://stillpub.app"`,
			`href="https://postmortem.report"`,
			// the org and the public repos it lists
			`href="https://github.com/parallaxintelligencepartnership"`,
			`href="https://github.com/parallaxintelligencepartnership/itworks"`,
			`href="https://github.com/Parallax-Intelligence-Partnership-LLC/mailautopsy"`,
			`href="https://github.com/parallaxintelligencepartnership/weatherdesk"`,
			`href="https://github.com/parallaxintelligencepartnership/openscan-hub"`,
			// the record kept here
			`href="/wall/"`,
			`href="/api/entries.json"`,
			`href="https://github.com/parallaxintelligencepartnership/itworks-site#embedding-the-wall-on-another-site"`,
			// a heading in the notice voice, never the bare brand word
			"Also posted by this office",
		} {
			if !strings.Contains(foot, want) {
				t.Fatalf("%s footer is missing %q", name, want)
			}
		}

		// Link equity is the point of these links: none of them is dropped.
		if strings.Contains(foot, "nofollow") {
			t.Fatalf("%s footer marks an outbound link nofollow", name)
		}
		if strings.Contains(strings.ToLower(body), "linkedin") {
			t.Fatalf("%s carries a LinkedIn link", name)
		}
		if strings.Contains(foot, ">Parallax<") {
			t.Fatalf("%s footer uses the bare word Parallax as a heading", name)
		}
	}
}
