package entry

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// idPattern is the entry id: 12 characters of the lowercase base32
// alphabet, which is also the file name stem under entries/.
var idPattern = regexp.MustCompile(`^[a-z2-7]{12}$`)

// ValidID reports whether s is a well formed entry id.
func ValidID(s string) bool { return idPattern.MatchString(s) }

// Load reads every *.json file in dir, validates it, and returns the
// entries sorted by id. now is the reference time for the audit date rules.
// Warnings (a file whose add commit cannot be found, for instance) are
// written to warn; nil means os.Stderr.
//
// Every problem found is returned, not just the first, so one run can list
// everything that has to be fixed. Entries are returned only when the
// error slice is empty.
func Load(dir string, now time.Time, warn io.Writer) ([]Entry, []error) {
	if warn == nil {
		warn = os.Stderr
	}

	if err := checkNotShallow(dir); err != nil {
		return nil, []error{err}
	}

	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, []error{fmt.Errorf("read %s: %w", dir, err)}
	}

	var paths []string
	for _, de := range dirEntries {
		name := de.Name()
		if de.IsDir() {
			fmt.Fprintf(warn, "warning: %s: subdirectory under entries/ is ignored\n", name)
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			fmt.Fprintf(warn, "warning: %s: non-json file under entries/ is ignored\n", name)
			continue
		}
		if de.Type()&os.ModeSymlink != 0 {
			fmt.Fprintf(warn, "warning: %s: symlink under entries/ is ignored\n", name)
			continue
		}
		if !de.Type().IsRegular() {
			fmt.Fprintf(warn, "warning: %s: non-regular file under entries/ is ignored\n", name)
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)

	var (
		errs    []error
		entries []Entry
		seen    = make(map[string]string, len(paths))
	)

	for _, path := range paths {
		base := filepath.Base(path)
		id := strings.TrimSuffix(base, ".json")

		if !ValidID(id) {
			errs = append(errs, fmt.Errorf("%s: file name must be 12 characters from a to z and 2 to 7, plus .json", base))
			continue
		}
		// The id is compared case-insensitively: two files whose stems
		// differ only in case are the same file on a case-insensitive
		// filesystem (macOS, Windows), and would collide if ever moved to
		// a case-sensitive one, so they collide here too.
		key := strings.ToLower(id)
		if first, dup := seen[key]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate entry id, already used by %s", base, first))
			continue
		}
		seen[key] = base

		f, err := os.Open(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", base, err))
			continue
		}
		rec, err := Decode(f)
		f.Close()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", base, err))
			continue
		}

		e, msg := Validate(rec, now)
		if msg != "" {
			errs = append(errs, fmt.Errorf("%s: %s", base, msg))
			continue
		}

		e.ID = id
		approved, err := approvedAt(dir, path, warn)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		e.ApprovedAt = approved
		entries = append(entries, e)
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return entries, nil
}

// checkNotShallow refuses a shallow checkout of dir before any entry is
// read. A shallow clone (GitHub Actions' default fetch-depth: 1, say) does
// not merely lack history: git treats its boundary commit as a root commit,
// so `git log --diff-filter=A` reports every file present there as freshly
// added on that commit's date. That is not an absence approvedAt's mtime
// fallback can catch — it is a wrong-but-present date — so it has to be
// caught up front, once, rather than per entry. If git is not installed or
// dir is not a repository, this is not the shallow case and Load proceeds
// as before.
func checkNotShallow(dir string) error {
	git, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	out, err := exec.Command(git, "-C", dir, "rev-parse", "--is-shallow-repository").Output()
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(out)) == "true" {
		return fmt.Errorf("entries checkout is shallow, so approval dates would be wrong; fetch full history (fetch-depth: 0)")
	}
	return nil
}

// approvedAt returns the commit date of the commit that added path, which
// is the moment the pull request was merged and the entry was approved. If
// git is not installed, the directory is not a repository, or the file is
// untracked (a local preview of an entry not yet committed), it falls back
// to the file's modification time and says so on warn. If the file is
// tracked but no adding commit can be found — a shallow checkout whose
// history does not reach back to it — that is not a local preview, so it
// is reported as an error instead of silently backdating the entry to
// whatever moment the checkout happened to write the file.
func approvedAt(dir, path string, warn io.Writer) (time.Time, error) {
	fallback := func(reason string) time.Time {
		mtime := time.Time{}
		if fi, err := os.Stat(path); err == nil {
			mtime = fi.ModTime().UTC()
		}
		fmt.Fprintf(warn, "warning: %s: %s; using the file modification time %s as the approval date\n",
			filepath.Base(path), reason, mtime.Format(time.RFC3339))
		return mtime
	}

	git, err := exec.LookPath("git")
	if err != nil {
		return fallback("git is not installed"), nil
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cmd := exec.Command(git, "-C", dir, "log", "--diff-filter=A", "--format=%cI", "-1", "--", abs)
	out, err := cmd.Output()
	if err != nil {
		return fallback("git could not read the history of this file"), nil
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		tracked := exec.Command(git, "-C", dir, "ls-files", "--error-unmatch", "--", abs)
		if trackErr := tracked.Run(); trackErr == nil {
			return time.Time{}, fmt.Errorf(
				"%s: this file is tracked by git but the checkout has no history for it (a shallow checkout?); the approval date cannot be determined",
				filepath.Base(path))
		}
		return fallback("the file has no commit that adds it, so it is not merged yet"), nil
	}
	t, err := time.Parse(time.RFC3339, line)
	if err != nil {
		return fallback("git returned a commit date that could not be parsed"), nil
	}
	return t.UTC(), nil
}
