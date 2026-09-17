package entry

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sampleJSON = `{
  "name": "MSP Sentinel",
  "summary": "Monitors managed service infrastructure.",
  "source": "closed",
  "repo_url": "",
  "audit_tier": "audit",
  "audit_date": "2026-09-07",
  "found": 56,
  "fixed": 56,
  "accepted": 0,
  "critical_open": 0,
  "critical_accepted": 0
}`

func writeEntry(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadReadsValidatesAndNamesEntries(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, "mspsentinel2.json", sampleJSON)
	writeEntry(t, dir, "notes.txt", "ignored")

	var warn bytes.Buffer
	entries, errs := Load(dir, testNow, &warn)
	if len(errs) != 0 {
		t.Fatalf("Load errors: %v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].ID != "mspsentinel2" {
		t.Fatalf("id = %q, want the file name stem", entries[0].ID)
	}
	if entries[0].Name != "MSP Sentinel" {
		t.Fatalf("name = %q", entries[0].Name)
	}
	// A temp dir is not a git repository, so the loader falls back to the
	// file modification time and says so.
	if entries[0].ApprovedAt.IsZero() {
		t.Fatalf("ApprovedAt is zero, want the file modification time")
	}
	if !strings.Contains(warn.String(), "mspsentinel2.json") {
		t.Fatalf("expected a warning about the missing commit, got %q", warn.String())
	}
}

func TestLoadWarnsAndSkipsSubdirectoriesAndNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	writeEntry(t, dir, "mspsentinel2.json", sampleJSON)
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	writeEntry(t, filepath.Join(dir, "sub"), "iiiiiiiiiiii.json", sampleJSON)

	var warn bytes.Buffer
	entries, errs := Load(dir, testNow, &warn)
	if len(errs) != 0 {
		t.Fatalf("Load errors: %v", errs)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (subdirectory must not be read)", len(entries))
	}
	if !strings.Contains(warn.String(), "sub") {
		t.Fatalf("expected a warning naming the skipped subdirectory, got %q", warn.String())
	}
}

func TestLoadRejectsBadFileNamesAndReportsEveryError(t *testing.T) {
	dir := t.TempDir()
	// The file names differ in more than case: a macOS temp directory is
	// case insensitive, so two names that differ only in case are one file.
	writeEntry(t, dir, "MSPSENTINELX.json", sampleJSON)             // uppercase
	writeEntry(t, dir, "short.json", sampleJSON)                    // too short
	writeEntry(t, dir, "abcdefghijk1.json", sampleJSON)             // 1 is not base32
	writeEntry(t, dir, "mspsentinel2.json", `{"name":"only name"}`) // invalid body

	_, errs := Load(dir, testNow, &bytes.Buffer{})
	if len(errs) != 4 {
		t.Fatalf("errors = %d (%v), want 4", len(errs), errs)
	}
}

func TestLoadRejectsAnIDInsideTheJSON(t *testing.T) {
	dir := t.TempDir()
	body := strings.Replace(sampleJSON, `{`, `{"id":"mspsentinel2",`, 1)
	writeEntry(t, dir, "mspsentinel2.json", body)

	_, errs := Load(dir, testNow, &bytes.Buffer{})
	if len(errs) != 1 {
		t.Fatalf("errors = %v, want one error about the id field", errs)
	}
}

func TestLoadUsesTheAddingCommitDate(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_COMMITTER_DATE=2026-03-04T05:06:07Z", "GIT_AUTHOR_DATE=2026-03-04T05:06:07Z",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init", "-q")
	// entries/ is a subdirectory of the git repository, the same layout as
	// production, so the repository's own .git directory never sits inside
	// the directory Load reads.
	entriesDir := filepath.Join(dir, "entries")
	if err := os.Mkdir(entriesDir, 0o755); err != nil {
		t.Fatalf("mkdir entries: %v", err)
	}
	writeEntry(t, entriesDir, "mspsentinel2.json", sampleJSON)
	run("add", "entries/mspsentinel2.json")
	run("commit", "-q", "-m", "add an entry")

	var warn bytes.Buffer
	entries, errs := Load(entriesDir, testNow, &warn)
	if len(errs) != 0 {
		t.Fatalf("Load errors: %v", errs)
	}
	want := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	if !entries[0].ApprovedAt.Equal(want) {
		t.Fatalf("ApprovedAt = %s, want %s", entries[0].ApprovedAt, want)
	}
	if warn.Len() != 0 {
		t.Fatalf("unexpected warning: %s", warn.String())
	}
}
