package main

import (
	"math"
	"time"

	"itworks.dev/internal/badge"
	"itworks.dev/internal/entry"
)

// baseURL is where the built site is published. It only appears in the
// values that have to work away from the site: the badge URL a README
// embeds and the entry URL the JSON feed publishes. Everything inside a
// page uses a root relative path so a local preview works too.
const baseURL = "https://itworks.dev"

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
}

func toView(e entry.Entry, today time.Time) view {
	auditDate, _ := time.Parse("2006-01-02", e.AuditDate)
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
		BadgeColor:       badge.Color(auditDate, e.Critical(), today),
		BadgeURL:         baseURL + "/badge/" + e.ID + ".svg",
		EntryURL:         baseURL + "/e/" + e.ID + "/",
		BadgePath:        "/badge/" + e.ID + ".svg",
		EntryPath:        "/e/" + e.ID + "/",
	}
}

func ageInDays(auditDate, now time.Time) int {
	a := time.Date(auditDate.UTC().Year(), auditDate.UTC().Month(), auditDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(n.Sub(a).Hours() / 24))
}

// landingPreviewCount is how many entries the landing page shows in its
// wall preview.
const landingPreviewCount = 3

type landingData struct {
	Title           string
	InstallCommands []string
	Entries         []view
}

type wallData struct {
	Title   string
	Entries []view
}

type entryData struct {
	Title string
	Entry view
}

type pageData struct {
	Title string
}

// installCommands is the exact, fixed pair of commands shown on the
// landing page.
var installCommands = []string{
	"claude plugin marketplace add parallaxintelligencepartnership/itworks",
	"claude plugin install itworks@itworks",
}
