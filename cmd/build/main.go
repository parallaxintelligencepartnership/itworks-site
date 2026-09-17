// Command build renders itworks.dev into a directory of static files.
//
//	go run ./cmd/build -out dist          write the site
//	go run ./cmd/build -check             validate entries/ and write nothing
//	go run ./cmd/build -today 2026-01-31  render as if today were that date
//
// The entries directory is the record: one JSON file per entry, named by
// its id. A merged pull request is an approval; deleting the file hides
// the entry on the next build.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"itworks.dev/internal/badge"
	"itworks.dev/internal/entry"
	"itworks.dev/web"
)

// domain is the custom domain GitHub Pages serves, written to CNAME.
const domain = "itworks.dev"

func main() {
	var (
		outDir     = flag.String("out", "dist", "directory to write the site into")
		entriesDir = flag.String("entries", "entries", "directory holding the entry JSON files")
		check      = flag.Bool("check", false, "validate the entries and write nothing")
		todayFlag  = flag.String("today", "", "reference date as YYYY-MM-DD, default today in UTC")
	)
	flag.Parse()

	if err := run(*entriesDir, *outDir, *todayFlag, *check, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run loads the entries and, unless checking only, renders the site.
func run(entriesDir, outDir, todayFlag string, check bool, warn io.Writer) error {
	today, err := referenceDate(todayFlag)
	if err != nil {
		return err
	}

	entries, errs := entry.Load(entriesDir, today, warn)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(warn, "error: %v\n", e)
		}
		return fmt.Errorf("%d of the entries in %s are not valid; nothing was written", len(errs), entriesDir)
	}
	if check {
		fmt.Fprintf(warn, "checked %d entries in %s, all valid\n", len(entries), entriesDir)
		return nil
	}

	return render(entries, outDir, today)
}

// referenceDate turns the -today flag into the UTC midnight the badge
// colors and the audit date rules are measured against.
func referenceDate(s string) (time.Time, error) {
	if s == "" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("today must be a date formatted as year, month, day: %w", err)
	}
	return t.UTC(), nil
}

// render writes every file of the site into outDir.
func render(entries []entry.Entry, outDir string, today time.Time) error {
	// Newest approval first; two entries approved in the same second are
	// ordered by id so the wall is the same on every build.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ApprovedAt.Equal(entries[j].ApprovedAt) {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].ApprovedAt.After(entries[j].ApprovedAt)
	})

	views := make([]view, 0, len(entries))
	for _, e := range entries {
		views = append(views, toView(e, today))
	}

	tpl, err := template.ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	if err := cleanOutDir(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	preview := views
	if len(preview) > landingPreviewCount {
		preview = preview[:landingPreviewCount]
	}

	pages := []struct {
		path string
		tmpl string
		data any
	}{
		{"index.html", "landing.html", landingData{
			Title:           "itworks.dev",
			InstallCommands: installCommands,
			Entries:         preview,
		}},
		{filepath.Join("wall", "index.html"), "wall.html", wallData{Title: "The wall", Entries: views}},
		{"404.html", "notfound.html", pageData{Title: "Page not found"}},
	}
	for _, e := range views {
		pages = append(pages, struct {
			path string
			tmpl string
			data any
		}{filepath.Join("e", e.ID, "index.html"), "entry.html", entryData{Title: e.Name, Entry: e}})
	}

	for _, p := range pages {
		var buf bytes.Buffer
		if err := tpl.ExecuteTemplate(&buf, p.tmpl, p.data); err != nil {
			return fmt.Errorf("render %s: %w", p.path, err)
		}
		if err := writeFile(outDir, p.path, buf.Bytes()); err != nil {
			return err
		}
	}

	for _, e := range views {
		svg := badge.Render(e.AuditDate, e.CriticalOpen+e.CriticalAccepted, e.BadgeColor)
		if err := writeFile(outDir, filepath.Join("badge", e.ID+".svg"), svg); err != nil {
			return err
		}
	}
	if err := writeExampleBadges(outDir, today); err != nil {
		return err
	}

	feed, err := json.MarshalIndent(map[string]any{"entries": views}, "", "  ")
	if err != nil {
		return fmt.Errorf("render the entries feed: %w", err)
	}
	if err := writeFile(outDir, filepath.Join("api", "entries.json"), append(feed, '\n')); err != nil {
		return err
	}

	if err := copyStatic(outDir); err != nil {
		return err
	}
	if err := writeFile(outDir, "CNAME", []byte(domain+"\n")); err != nil {
		return err
	}
	// GitHub Pages runs Jekyll over an artifact unless this file is there.
	return writeFile(outDir, ".nojekyll", nil)
}

// ownedOutputPaths are the files and directories render ever writes,
// relative to outDir. cleanOutDir removes only these, so it never touches
// anything a maintainer might have placed in outDir by hand.
var ownedOutputPaths = []string{
	"index.html", "404.html", "CNAME", ".nojekyll",
	"wall", "e", "badge", "api", "static",
}

// cleanOutDir removes everything render owns from a previous build, so an
// entry that was removed from entries/ does not leave its page or badge
// behind on the next build. It only ever removes the specific files and
// directories render writes, and only when outDir is non-empty and not
// "/" or the filesystem root.
func cleanOutDir(outDir string) error {
	if outDir == "" || outDir == "/" || outDir == string(filepath.Separator) {
		return nil
	}
	for _, name := range ownedOutputPaths {
		if err := os.RemoveAll(filepath.Join(outDir, name)); err != nil {
			return fmt.Errorf("clean %s: %w", filepath.Join(outDir, name), err)
		}
	}
	return nil
}

// exampleBadges are the three plates the landing page shows as specimens.
// They are files rather than inline markup so no template ever has to be
// handed markup that skips contextual escaping.
var exampleBadges = []struct {
	name    string
	color   string
	ageDays int
	crit    int
}{
	{"example-green.svg", badge.ColorGreen, 0, 0},
	{"example-amber.svg", badge.ColorAmber, 40, 0},
	{"example-red.svg", badge.ColorRed, 0, 1},
}

func writeExampleBadges(outDir string, today time.Time) error {
	for _, b := range exampleBadges {
		date := today.AddDate(0, 0, -b.ageDays).Format("2006-01-02")
		if err := writeFile(outDir, filepath.Join("badge", b.name), badge.Render(date, b.crit, b.color)); err != nil {
			return err
		}
	}
	return nil
}

// copyStatic copies web/static into outDir/static.
func copyStatic(outDir string) error {
	return fs.WalkDir(web.FS, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(web.FS, path)
		if err != nil {
			return err
		}
		return writeFile(outDir, filepath.FromSlash(path), b)
	})
}

func writeFile(outDir, rel string, data []byte) error {
	path := filepath.Join(outDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
