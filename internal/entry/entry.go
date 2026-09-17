// Package entry holds the record that goes on the wall: the JSON file
// contract, its validation rules, and the loader that reads the entries
// directory. One file under entries/ is one entry; the file name is the id.
package entry

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Record mirrors one entries/<id>.json file. Every field is a pointer so a
// missing JSON key can be told apart from an explicit zero value (0 and ""
// are valid values for several fields).
type Record struct {
	Name             *string `json:"name"`
	Summary          *string `json:"summary"`
	Source           *string `json:"source"`
	RepoURL          *string `json:"repo_url"`
	AuditTier        *string `json:"audit_tier"`
	AuditDate        *string `json:"audit_date"`
	Found            *int    `json:"found"`
	Fixed            *int    `json:"fixed"`
	Accepted         *int    `json:"accepted"`
	CriticalOpen     *int    `json:"critical_open"`
	CriticalAccepted *int    `json:"critical_accepted"`
}

// Entry is one validated record, ready to render. ID comes from the file
// name and ApprovedAt from the commit that added the file.
type Entry struct {
	ID               string
	Name             string
	Summary          string
	Source           string
	RepoURL          string
	AuditTier        string
	AuditDate        string
	Found            int
	Fixed            int
	Accepted         int
	CriticalOpen     int
	CriticalAccepted int
	ApprovedAt       time.Time
}

// Critical is the count the badge reads: what is still open plus what the
// owner accepted and chose not to fix. An accepted critical is not a fixed
// critical, so it keeps the plate red.
func (e Entry) Critical() int { return e.CriticalOpen + e.CriticalAccepted }

const (
	minAuditYear         = "2025-01-01"
	maxNameLen           = 60
	maxSummaryLen        = 160
	maxRepoURLLen        = 200
	maxCounterVal        = 9999
	dateLayoutISO8601Day = "2006-01-02"
)

// errTrailingData means the file held more than one JSON value.
var errTrailingData = errors.New("file must contain a single JSON object")

// Decode reads and decodes one entry file, rejecting unknown fields, syntax
// errors, and trailing data. The id lives in the file name, so an "id" key
// inside the JSON is an unknown field and is rejected here.
func Decode(r io.Reader) (Record, error) {
	var rec Record
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rec); err != nil {
		return Record{}, err
	}
	if dec.More() {
		return Record{}, errTrailingData
	}
	return rec, nil
}

// Validate checks every field per the rules table in docs/SPEC.md and, on
// success, returns the Entry (without ID or ApprovedAt, which the loader
// fills in). On failure the returned message is plain English.
func Validate(rec Record, now time.Time) (Entry, string) {
	var e Entry

	if rec.Name == nil {
		return e, "name is required"
	}
	name := strings.TrimSpace(*rec.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameLen {
		return e, "name must be between 1 and 60 characters"
	}
	if !isPrintable(name) {
		return e, "name must be printable text with no control characters"
	}

	if rec.Summary == nil {
		return e, "summary is required"
	}
	summary := strings.TrimSpace(*rec.Summary)
	if n := utf8.RuneCountInString(summary); n < 1 || n > maxSummaryLen {
		return e, "summary must be between 1 and 160 characters"
	}
	if !isPrintable(summary) {
		return e, "summary must be printable text with no control characters"
	}

	if rec.Source == nil {
		return e, "source is required"
	}
	source := *rec.Source
	if source != "public" && source != "closed" {
		return e, "source must be public or closed"
	}

	if rec.RepoURL == nil {
		return e, `repo_url is required; use "" when source is closed`
	}
	repoURL := *rec.RepoURL
	if repoURL != "" {
		if source == "closed" {
			return e, "repo url must be empty when source is closed"
		}
		if len(repoURL) > maxRepoURLLen {
			return e, "repo url is too long"
		}
		if !isPrintable(repoURL) {
			return e, "repo url must be printable text with no control characters"
		}
		u, err := url.Parse(repoURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return e, "repo url must be a valid http or https url"
		}
		if u.User != nil {
			return e, "repo url must not carry a username or password"
		}
		if u.Fragment != "" || strings.Contains(repoURL, "#") {
			return e, "repo url must not carry a fragment"
		}
	}

	if rec.AuditTier == nil {
		return e, "audit tier is required"
	}
	tier := *rec.AuditTier
	if tier != "checkpoint" && tier != "closeout" && tier != "audit" {
		return e, "audit tier must be checkpoint, closeout, or audit"
	}

	if rec.AuditDate == nil {
		return e, "audit date is required"
	}
	dateStr := *rec.AuditDate
	auditDate, err := time.Parse(dateLayoutISO8601Day, dateStr)
	if err != nil || auditDate.Format(dateLayoutISO8601Day) != dateStr {
		return e, "audit date must be a valid calendar date formatted as year, month, day"
	}
	minDate, _ := time.Parse(dateLayoutISO8601Day, minAuditYear)
	todayUTC := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if auditDate.Before(minDate) {
		return e, "audit date must not be earlier than January 1, 2025"
	}
	if auditDate.After(todayUTC) {
		return e, "audit date must not be after today"
	}

	if rec.Found == nil {
		return e, "found is required"
	}
	if rec.Fixed == nil {
		return e, "fixed is required"
	}
	if rec.Accepted == nil {
		return e, "accepted is required"
	}
	if rec.CriticalOpen == nil {
		return e, "critical open is required"
	}
	if rec.CriticalAccepted == nil {
		return e, "critical accepted is required"
	}
	found, fixed, accepted := *rec.Found, *rec.Fixed, *rec.Accepted
	criticalOpen, criticalAccepted := *rec.CriticalOpen, *rec.CriticalAccepted

	if found < 0 || found > maxCounterVal {
		return e, "found must be between 0 and 9999"
	}
	if fixed < 0 || fixed > maxCounterVal {
		return e, "fixed must be between 0 and 9999"
	}
	if accepted < 0 || accepted > maxCounterVal {
		return e, "accepted must be between 0 and 9999"
	}
	if criticalOpen < 0 || criticalOpen > maxCounterVal {
		return e, "critical open must be between 0 and 9999"
	}
	if criticalAccepted < 0 || criticalAccepted > maxCounterVal {
		return e, "critical accepted must be between 0 and 9999"
	}
	if fixed+accepted > found {
		return e, "fixed plus accepted must not exceed found"
	}
	if criticalOpen > found {
		return e, "critical open must not exceed found"
	}
	if criticalOpen+criticalAccepted > found {
		return e, "critical open plus critical accepted must not exceed found"
	}
	if criticalAccepted > accepted {
		return e, "critical_accepted must not exceed accepted"
	}

	e = Entry{
		Name: name, Summary: summary, Source: source, RepoURL: repoURL,
		AuditTier: tier, AuditDate: dateStr,
		Found: found, Fixed: fixed, Accepted: accepted,
		CriticalOpen: criticalOpen, CriticalAccepted: criticalAccepted,
	}
	return e, ""
}

// isPrintable rejects control characters (category Cc), format characters
// (category Cf), and other invisible or non-space-bar whitespace runes. Cf
// carries the invisible runes that let a name read as one thing and store
// as another: the bidi overrides and isolates U+202A to U+202E and U+2066
// to U+2069, the zero width joiners and marks U+200B to U+200F, and the
// byte order mark U+FEFF. Also rejected: U+00A0 (no-break space), U+3164
// (Hangul filler), U+115F and U+1160 (Hangul choseong/jungseong fillers),
// U+FE0F (variation selector-16), U+2800 (braille pattern blank), every
// rune in the line and paragraph separator categories Zl and Zp, and any
// rune for which unicode.IsSpace is true other than U+0020, the ordinary
// space bar. Letters with combining marks and emoji stay legal.
func isPrintable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
		if r >= 0x202A && r <= 0x202E {
			return false
		}
		if r >= 0x2066 && r <= 0x2069 {
			return false
		}
		if r >= 0x200B && r <= 0x200F {
			return false
		}
		if r == 0xFEFF {
			return false
		}
		switch r {
		case 0x00A0, 0x3164, 0x115F, 0x1160, 0xFE0F, 0x2800:
			return false
		}
		if unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
		if unicode.IsSpace(r) && r != 0x0020 {
			return false
		}
	}
	return true
}
