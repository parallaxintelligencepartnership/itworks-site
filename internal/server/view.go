package server

import (
	"html/template"
	"math"
	"time"

	"itworks.dev/internal/badge"
	"itworks.dev/internal/store"
)

// EntryView is the shape handed to templates and to the JSON API.
type EntryView struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Summary      string `json:"summary"`
	Source       string `json:"source"`
	RepoURL      string `json:"repo_url"`
	AuditTier    string `json:"audit_tier"`
	AuditDate    string `json:"audit_date"`
	Found        int    `json:"found"`
	Fixed        int    `json:"fixed"`
	Accepted     int    `json:"accepted"`
	CriticalOpen int    `json:"critical_open"`
	AgeDays      int    `json:"age_days"`
	BadgeColor   string `json:"badge_color"`
	BadgeURL     string `json:"badge_url"`
	EntryURL     string `json:"entry_url"`
}

type LandingData struct {
	Title      string
	EntryCount int
	Badges     struct {
		Green, Amber, Red template.HTML
	}
	InstallCommands []string
}

type WallData struct {
	Title   string
	Entries []EntryView
}

type EntryData struct {
	Title string
	Entry EntryView
}

type AdminData struct {
	Title    string
	Pending  []EntryView
	Approved []EntryView
	Hidden   []EntryView
}

// InstallCommands is the exact, fixed pair of commands shown on the landing
// page.
var InstallCommands = []string{
	"claude plugin marketplace add parallaxintelligencepartnership/vibecheck",
	"claude plugin install vibecheck@vibecheck",
}

func (s *Server) badgeURL(id string) string {
	return s.baseURL + "/badge/" + id + ".svg"
}

func (s *Server) entryURL(id string) string {
	return s.baseURL + "/e/" + id
}

func (s *Server) toEntryView(e store.Entry, now time.Time) EntryView {
	auditDate, _ := time.Parse("2006-01-02", e.AuditDate)
	ageDays := ageInDays(auditDate, now)
	color := badge.Color(auditDate, e.CriticalOpen, now)

	return EntryView{
		ID:           e.ID,
		Name:         e.Name,
		Summary:      e.Summary,
		Source:       e.Source,
		RepoURL:      e.RepoURL,
		AuditTier:    e.AuditTier,
		AuditDate:    e.AuditDate,
		Found:        e.Found,
		Fixed:        e.Fixed,
		Accepted:     e.Accepted,
		CriticalOpen: e.CriticalOpen,
		AgeDays:      ageDays,
		BadgeColor:   color,
		BadgeURL:     s.badgeURL(e.ID),
		EntryURL:     s.entryURL(e.ID),
	}
}

func ageInDays(auditDate, now time.Time) int {
	a := time.Date(auditDate.UTC().Year(), auditDate.UTC().Month(), auditDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(math.Round(n.Sub(a).Hours() / 24))
}

func (s *Server) toEntryViews(entries []store.Entry, now time.Time) []EntryView {
	out := make([]EntryView, 0, len(entries))
	for _, e := range entries {
		out = append(out, s.toEntryView(e, now))
	}
	return out
}

// sampleBadges renders one representative badge SVG per color for the
// landing page: a fresh green, a stale amber, and a red with a critical
// still open.
func sampleBadges(now time.Time) (green, amber, red template.HTML) {
	today := now.UTC().Format("2006-01-02")
	stale := now.UTC().AddDate(0, 0, -40).Format("2006-01-02")
	green = template.HTML(badge.Render(today, 0, badge.ColorGreen))
	amber = template.HTML(badge.Render(stale, 0, badge.ColorAmber))
	red = template.HTML(badge.Render(today, 1, badge.ColorRed))
	return
}
