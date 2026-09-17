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
		e.ApprovedAt = approvedAt(dir, path, warn)
		entries = append(entries, e)
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return entries, nil
}

// approvedAt returns the commit date of the commit that added path, which
// is the moment the pull request was merged and the entry was approved. If
// git is not installed, the directory is not a repository, or the file was
// never committed, it falls back to the file's modification time and says
// so on warn.
func approvedAt(dir, path string, warn io.Writer) time.Time {
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
		return fallback("git is not installed")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	cmd := exec.Command(git, "-C", dir, "log", "--diff-filter=A", "--format=%cI", "-1", "--", abs)
	out, err := cmd.Output()
	if err != nil {
		return fallback("git could not read the history of this file")
	}
	line := strings.TrimSpace(string(out))
	if line == "" {
		return fallback("the file has no commit that adds it, so it is not merged yet")
	}
	t, err := time.Parse(time.RFC3339, line)
	if err != nil {
		return fallback("git returned a commit date that could not be parsed")
	}
	return t.UTC()
}
