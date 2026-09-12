package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"itworks.dev/internal/store"
)

// submitRequest mirrors the POST /api/entries body. Every field is a
// pointer so a missing JSON key can be told apart from an explicit zero
// value (0, "" are valid values for several fields).
type submitRequest struct {
	Name         *string `json:"name"`
	Summary      *string `json:"summary"`
	Source       *string `json:"source"`
	RepoURL      *string `json:"repo_url"`
	AuditTier    *string `json:"audit_tier"`
	AuditDate    *string `json:"audit_date"`
	Found        *int    `json:"found"`
	Fixed        *int    `json:"fixed"`
	Accepted     *int    `json:"accepted"`
	CriticalOpen *int    `json:"critical_open"`
}

const (
	minAuditYear         = "2025-01-01"
	maxNameLen           = 60
	maxSummaryLen        = 160
	maxRepoURLLen        = 200
	maxCounterVal        = 9999
	dateLayoutISO8601Day = "2006-01-02"
)

// errTrailingData means the body held more than one JSON value.
var errTrailingData = errors.New("request body must contain a single JSON object")

// decodeSubmitRequest reads and decodes the request body, rejecting unknown
// fields, syntax errors, and trailing data. A *http.MaxBytesError bubbles up
// unwrapped so the caller can tell an over cap body apart from a malformed
// one; any other error should map to a 400.
func decodeSubmitRequest(body io.Reader) (submitRequest, error) {
	var req submitRequest
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return submitRequest{}, err
	}
	if dec.More() {
		return submitRequest{}, errTrailingData
	}
	return req, nil
}

// validateSubmit checks every field per the validation table and, on
// success, returns a store.NewEntry ready to insert. On failure the
// returned message is plain English and safe to send back as the error
// field of a 400 response.
func validateSubmit(req submitRequest, now time.Time) (store.NewEntry, string) {
	var ne store.NewEntry

	if req.Name == nil {
		return ne, "name is required"
	}
	name := strings.TrimSpace(*req.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameLen {
		return ne, "name must be between 1 and 60 characters"
	}
	if !isPrintable(name) {
		return ne, "name must be printable text with no control characters"
	}

	if req.Summary == nil {
		return ne, "summary is required"
	}
	summary := strings.TrimSpace(*req.Summary)
	if n := utf8.RuneCountInString(summary); n < 1 || n > maxSummaryLen {
		return ne, "summary must be between 1 and 160 characters"
	}
	if !isPrintable(summary) {
		return ne, "summary must be printable text with no control characters"
	}

	if req.Source == nil {
		return ne, "source is required"
	}
	source := *req.Source
	if source != "public" && source != "closed" {
		return ne, "source must be public or closed"
	}

	repoURL := ""
	if req.RepoURL != nil {
		repoURL = *req.RepoURL
	}
	if repoURL != "" {
		if source == "closed" {
			return ne, "repo url must be empty when source is closed"
		}
		if len(repoURL) > maxRepoURLLen {
			return ne, "repo url is too long"
		}
		u, err := url.Parse(repoURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return ne, "repo url must be a valid https url"
		}
	}

	if req.AuditTier == nil {
		return ne, "audit tier is required"
	}
	tier := *req.AuditTier
	if tier != "checkpoint" && tier != "closeout" && tier != "audit" {
		return ne, "audit tier must be checkpoint, closeout, or audit"
	}

	if req.AuditDate == nil {
		return ne, "audit date is required"
	}
	dateStr := *req.AuditDate
	auditDate, err := time.Parse(dateLayoutISO8601Day, dateStr)
	if err != nil || auditDate.Format(dateLayoutISO8601Day) != dateStr {
		return ne, "audit date must be a valid calendar date formatted as year, month, day"
	}
	minDate, _ := time.Parse(dateLayoutISO8601Day, minAuditYear)
	todayUTC := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	if auditDate.Before(minDate) {
		return ne, "audit date must not be earlier than January 1, 2025"
	}
	if auditDate.After(todayUTC) {
		return ne, "audit date must not be after today"
	}

	if req.Found == nil {
		return ne, "found is required"
	}
	if req.Fixed == nil {
		return ne, "fixed is required"
	}
	if req.Accepted == nil {
		return ne, "accepted is required"
	}
	if req.CriticalOpen == nil {
		return ne, "critical open is required"
	}
	found, fixed, accepted, criticalOpen := *req.Found, *req.Fixed, *req.Accepted, *req.CriticalOpen

	if found < 0 || found > maxCounterVal {
		return ne, "found must be between 0 and 9999"
	}
	if fixed < 0 || fixed > maxCounterVal {
		return ne, "fixed must be between 0 and 9999"
	}
	if accepted < 0 || accepted > maxCounterVal {
		return ne, "accepted must be between 0 and 9999"
	}
	if criticalOpen < 0 || criticalOpen > maxCounterVal {
		return ne, "critical open must be between 0 and 9999"
	}
	if fixed+accepted > found {
		return ne, "fixed plus accepted must not exceed found"
	}
	if criticalOpen > found {
		return ne, "critical open must not exceed found"
	}

	ne = store.NewEntry{
		Name: name, Summary: summary, Source: source, RepoURL: repoURL,
		AuditTier: tier, AuditDate: dateStr,
		Found: found, Fixed: fixed, Accepted: accepted, CriticalOpen: criticalOpen,
	}
	return ne, ""
}

func isPrintable(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
