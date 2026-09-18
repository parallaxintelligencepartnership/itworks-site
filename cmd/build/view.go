package main

import (
	"fmt"
	"math"
	"time"

	"itworks.build/internal/badge"
	"itworks.build/internal/entry"
)

// baseURL is where the built site is published. It appears in the values
// that have to work away from the site (the badge URL a README embeds, the
// entry URL the JSON feed publishes) and in the head of every page, which
// has to name its own absolute address for a crawler. Everything else
// inside a page uses a root relative path so a local preview works too.
const baseURL = "https://itworks.build"

// view is one entry as the templates and the JSON feed see it. The JSON
// field set is the one the old API returned, plus critical_accepted.
type view struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Summary          string `json:"summary"`
	Source           string `json:"source"`
	RepoURL          string `json:"repo_url"`
	AuditTier        string `json:"audit_tier"`
	AuditDate        string `json:"audit_date"`
	Found            int    `json:"found"`
	Fixed            int    `json:"fixed"`
	Accepted         int    `json:"accepted"`
	CriticalOpen     int    `json:"critical_open"`
	CriticalAccepted int    `json:"critical_accepted"`
	CriticalTotal    int    `json:"-"`
	AgeDays          int    `json:"age_days"`
	BadgeColor       string `json:"badge_color"`
	BadgeURL         string `json:"badge_url"`
	EntryURL         string `json:"entry_url"`

	// Paths are for markup inside the site and stay out of the feed.
	BadgePath string `json:"-"`
	EntryPath string `json:"-"`

	// State is the entry's state in words, and Flap is the same word set
	// in split flap cells. Both stay out of the feed: they are a reading
	// of badge_color, not a field of the record.
	State string `json:"-"`
	Flap  flap   `json:"-"`

	// ApprovedAt is the merge date of the pull request that added the
	// entry file; the sitemap publishes it as lastmod.
	ApprovedAt time.Time `json:"-"`
}

func toView(e entry.Entry, today time.Time) view {
	auditDate, _ := time.Parse("2006-01-02", e.AuditDate)
	color := badge.Color(auditDate, e.Critical(), today)
	return view{
		ID:               e.ID,
		Name:             e.Name,
		Summary:          e.Summary,
		Source:           e.Source,
		RepoURL:          e.RepoURL,
		AuditTier:        e.AuditTier,
		AuditDate:        e.AuditDate,
		Found:            e.Found,
		Fixed:            e.Fixed,
		Accepted:         e.Accepted,
		CriticalOpen:     e.CriticalOpen,
		CriticalAccepted: e.CriticalAccepted,
		CriticalTotal:    e.CriticalOpen + e.CriticalAccepted,
		AgeDays:          ageInDays(auditDate, today),
		BadgeColor:       color,
		BadgeURL:         baseURL + "/badge/" + e.ID + ".svg",
		EntryURL:         baseURL + "/e/" + e.ID + "/",
		BadgePath:        "/badge/" + e.ID + ".svg",
		EntryPath:        "/e/" + e.ID + "/",
		State:            stateWords(color),
		Flap:             stateFlap(color),
		ApprovedAt:       e.ApprovedAt,
	}
}

func ageInDays(auditDate, now time.Time) int {
	a := time.Date(auditDate.UTC().Year(), auditDate.UTC().Month(), auditDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(n.Sub(a).Hours() / 24))
}

// stateWords is the state a visitor reads, spelled out. The badge color is
// never the only carrier of the state on a page.
func stateWords(color string) string {
	switch color {
	case badge.ColorRed:
		return "critical open"
	case badge.ColorAmber:
		return "stale, over 30 days"
	default:
		return "current"
	}
}

// flap is a word on the departure board: one cell per letter, plus the
// class that colors the cells and the label a screen reader is given
// instead of the cells.
type flap struct {
	Class string
	Label string
	Cells []flapCell
}

// flapCell is one cell. A space in the word is a gap between cells, not a
// cell with a space in it.
type flapCell struct {
	Char  string
	Space bool
}

func stateFlap(color string) flap {
	switch color {
	case badge.ColorRed:
		return newFlap("f-crit", "CRITICAL OPEN")
	case badge.ColorAmber:
		return newFlap("f-stale", "STALE")
	default:
		return newFlap("f-cur", "CURRENT")
	}
}

func newFlap(class, word string) flap {
	f := flap{Class: class, Label: word}
	for _, r := range word {
		if r == ' ' {
			f.Cells = append(f.Cells, flapCell{Space: true})
			continue
		}
		f.Cells = append(f.Cells, flapCell{Char: string(r)})
	}
	return f
}

// landingPreviewCount is how many entries the landing page shows in its
// register preview.
const landingPreviewCount = 3

// meta is the head of a page: what it is called, when it was built, and
// which bar link is current. Every page data struct embeds it.
type meta struct {
	Title string
	// BuildDate is the day this copy of the site was rendered, printed on
	// the notice header as the posted date.
	BuildDate string
	// Nav names the bar link that is current: "" or "wall".
	Nav string
}

type landingData struct {
	meta
	NoticeNo        string
	InstallCommands []installTab
	Entries         []view
	Rows            []row
	States          []stateSpec
	Published       []string
}

type wallData struct {
	meta
	Rows []row
}

// row is one line of the register: the slot number the notice is posted
// in, and the notice itself. The board is numbered from the top, so the
// newest entry is 001.
type row struct {
	No    string
	Entry view
}

func rows(views []view) []row {
	out := make([]row, 0, len(views))
	for i, v := range views {
		out = append(out, row{No: fmt.Sprintf("%03d", i+1), Entry: v})
	}
	return out
}

type entryData struct {
	meta
	Entry view
}

type pageData struct {
	meta
}

// stateSpec is one row of Board A on the landing page: the state word on
// the flaps, the specimen plate, and the rule that puts an entry there.
type stateSpec struct {
	Flap      flap
	BadgePath string
	BadgeAlt  string
	Lead      string
	Rest      string
}

// boardStates is Board A, in the order a badge walks through them.
var boardStates = []stateSpec{
	{
		Flap:      newFlap("f-cur", "CURRENT"),
		BadgePath: "/badge/example-green.svg",
		BadgeAlt:  "Example audit badge: green, audited today, 0 critical open",
		Lead:      "Audited inside 30 days, nothing critical open.",
		Rest:      "The plate stays green until one of those two stops being true.",
	},
	{
		Flap:      newFlap("f-stale", "STALE"),
		BadgePath: "/badge/example-amber.svg",
		BadgeAlt:  "Example audit badge: amber, audited 40 days ago, 0 critical open, stale",
		Lead:      "Day 30 turns the flap.",
		Rest:      "The word stale is printed inside the plate, so the state survives a grayscale screen.",
	},
	{
		Flap:      newFlap("f-crit", "CRITICAL OPEN"),
		BadgePath: "/badge/example-red.svg",
		BadgeAlt:  "Example audit badge: red, audited today, 1 critical open",
		Lead:      "Red on day one if it has to be.",
		Rest:      "One open critical beats a fresh audit date, and it stays red until that finding is closed.",
	},
}

// publishedFields is the register of what a closeout summary publishes,
// Schedule 1 on the landing page.
var publishedFields = []string{
	"name", "summary", "source", "audit",
	"found", "fixed", "accepted", "criticals open", "criticals accepted",
}

// installTab is one tear off tab under the perforated rule: its number,
// what tearing it off does, and the line to run.
type installTab struct {
	No      string
	What    string
	Command string
}

// installCommands is the exact, fixed pair of commands shown on the
// landing page, in the order they have to be run.
var installCommands = []installTab{
	{No: "Tab 01", What: "add the marketplace", Command: "claude plugin marketplace add parallaxintelligencepartnership/itworks"},
	{No: "Tab 02", What: "install the plugin", Command: "claude plugin install itworks@itworks"},
}
