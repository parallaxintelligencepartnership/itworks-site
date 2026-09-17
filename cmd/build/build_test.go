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
	if !strings.Contains(page, `<span class="lab">critical open</span><span class="v num">0</span>`) {
		t.Fatalf("entry page does not show 0 critical open:\n%s", page)
	}
	if !strings.Contains(page, `<span class="lab">critical accepted by the owner</span><span class="v num">1</span>`) {
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
	end := strings.Index(wall[i:], `<div class="badgecell">`)
	if end < 0 {
		t.Fatalf("wall row for acceptedcrit has no badgecell")
	}
	row := wall[i : i+end]
	if !strings.Contains(row, `<span class="k">critical</span><span class="v num">1</span> <span class="crit-accepted">(1 accepted)</span>`) {
		t.Fatalf("wall critical cell does not count the accepted critical:\n%s", row)
	}
	if strings.Contains(row, "crit-zero") {
		t.Fatalf("wall critical cell styles an accepted critical as zero:\n%s", row)
	}

	j := strings.Index(wall, "/e/cleanentry01/")
	if j < 0 {
		t.Fatalf("wall is missing cleanentry01")
	}
	endJ := strings.Index(wall[j:], `<div class="badgecell">`)
	if endJ < 0 {
		t.Fatalf("wall row for cleanentry01 has no badgecell")
	}
	rowJ := wall[j : j+endJ]
	if !strings.Contains(rowJ, `<span class="k">critical</span><span class="v num">0</span></p>`) {
		t.Fatalf("wall critical cell does not show 0 with nothing appended:\n%s", rowJ)
	}
	if strings.Contains(rowJ, "crit-accepted") {
		t.Fatalf("wall critical cell should not mention accepted for a clean entry:\n%s", rowJ)
	}
	if !strings.Contains(rowJ, "crit-zero") {
		t.Fatalf("wall critical cell does not style a clean entry as zero:\n%s", rowJ)
	}
}
