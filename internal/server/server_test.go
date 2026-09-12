package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"itworks.dev/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newTestServer builds a fresh Server (fresh db, fresh rate limiter) wrapped
// in an httptest.Server, so each test gets isolated state.
func newTestServer(t *testing.T, cfg Config) (*httptest.Server, *store.DB) {
	t.Helper()
	db := newTestDB(t)
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://itworks.dev"
	}
	srv, err := New(db, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.Routes())
	t.Cleanup(ts.Close)
	return ts, db
}

func validPayload() map[string]any {
	return map[string]any{
		"name":          "MSP Sentinel",
		"summary":       "closeout finished with no critical issues left open",
		"source":        "closed",
		"repo_url":      "",
		"audit_tier":    "audit",
		"audit_date":    "2026-09-07",
		"found":         56,
		"fixed":         56,
		"accepted":      0,
		"critical_open": 0,
	}
}

func postJSON(t *testing.T, url, contentType string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	return resp
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	return b
}

// --- validation table ---------------------------------------------------

func TestValidateSubmitTable(t *testing.T) {
	base := func() submitRequest {
		name, summary, source, tier, date := "MSP Sentinel", "did the closeout", "public", "audit", "2026-01-01"
		found, fixed, accepted, critical := 10, 10, 0, 0
		return submitRequest{
			Name: &name, Summary: &summary, Source: &source,
			AuditTier: &tier, AuditDate: &date,
			Found: &found, Fixed: &fixed, Accepted: &accepted, CriticalOpen: &critical,
		}
	}
	strp := func(s string) *string { return &s }
	intp := func(i int) *int { return &i }

	now := time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name    string
		mutate  func(submitRequest) submitRequest
		wantErr bool
	}{
		{"valid submission is accepted", func(r submitRequest) submitRequest { return r }, false},
		{"missing name", func(r submitRequest) submitRequest { r.Name = nil; return r }, true},
		{"name too long", func(r submitRequest) submitRequest { r.Name = strp(strings.Repeat("a", 61)); return r }, true},
		{"name empty after trim", func(r submitRequest) submitRequest { r.Name = strp("   "); return r }, true},
		{"name has control char", func(r submitRequest) submitRequest { r.Name = strp("bad\x01name"); return r }, true},
		{"missing summary", func(r submitRequest) submitRequest { r.Summary = nil; return r }, true},
		{"summary too long", func(r submitRequest) submitRequest { r.Summary = strp(strings.Repeat("a", 161)); return r }, true},
		{"missing source", func(r submitRequest) submitRequest { r.Source = nil; return r }, true},
		{"bad source value", func(r submitRequest) submitRequest { r.Source = strp("private"); return r }, true},
		{"repo url not https", func(r submitRequest) submitRequest {
			r.Source = strp("public")
			r.RepoURL = strp("http://example.com/a")
			return r
		}, true},
		{"repo url no host", func(r submitRequest) submitRequest {
			r.Source = strp("public")
			r.RepoURL = strp("https://")
			return r
		}, true},
		{"repo url too long", func(r submitRequest) submitRequest {
			r.Source = strp("public")
			r.RepoURL = strp("https://example.com/" + strings.Repeat("a", 200))
			return r
		}, true},
		{"repo url set when source closed", func(r submitRequest) submitRequest {
			r.Source = strp("closed")
			r.RepoURL = strp("https://example.com/a")
			return r
		}, true},
		{"repo url ok when source public", func(r submitRequest) submitRequest {
			r.Source = strp("public")
			r.RepoURL = strp("https://example.com/a")
			return r
		}, false},
		{"missing audit tier", func(r submitRequest) submitRequest { r.AuditTier = nil; return r }, true},
		{"bad audit tier", func(r submitRequest) submitRequest { r.AuditTier = strp("weekly"); return r }, true},
		{"missing audit date", func(r submitRequest) submitRequest { r.AuditDate = nil; return r }, true},
		{"bad audit date format", func(r submitRequest) submitRequest { r.AuditDate = strp("09/01/2026"); return r }, true},
		{"impossible calendar date", func(r submitRequest) submitRequest { r.AuditDate = strp("2026-02-30"); return r }, true},
		{"audit date before floor", func(r submitRequest) submitRequest { r.AuditDate = strp("2024-12-31"); return r }, true},
		{"audit date after today", func(r submitRequest) submitRequest { r.AuditDate = strp("2026-09-12"); return r }, true},
		{"audit date today is ok", func(r submitRequest) submitRequest { r.AuditDate = strp("2026-09-11"); return r }, false},
		{"missing found", func(r submitRequest) submitRequest { r.Found = nil; return r }, true},
		{"missing fixed", func(r submitRequest) submitRequest { r.Fixed = nil; return r }, true},
		{"missing accepted", func(r submitRequest) submitRequest { r.Accepted = nil; return r }, true},
		{"missing critical open", func(r submitRequest) submitRequest { r.CriticalOpen = nil; return r }, true},
		{"found too high", func(r submitRequest) submitRequest { r.Found = intp(10000); return r }, true},
		{"found negative", func(r submitRequest) submitRequest { r.Found = intp(-1); return r }, true},
		{"fixed plus accepted exceeds found", func(r submitRequest) submitRequest {
			r.Found = intp(10)
			r.Fixed = intp(8)
			r.Accepted = intp(5)
			return r
		}, true},
		{"critical open exceeds found", func(r submitRequest) submitRequest {
			r.Found = intp(2)
			r.CriticalOpen = intp(3)
			return r
		}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, msg := validateSubmit(c.mutate(base()), now)
			if c.wantErr && msg == "" {
				t.Fatalf("expected a validation error, got none")
			}
			if !c.wantErr && msg != "" {
				t.Fatalf("expected no error, got %q", msg)
			}
		})
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	body := []byte(`{"name":"A","summary":"s","source":"public","audit_tier":"audit","audit_date":"2026-01-01","found":1,"fixed":1,"accepted":0,"critical_open":0,"extra_field":"nope"}`)
	resp := postJSON(t, ts.URL+"/api/entries", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// --- content type, size cap, rate limit, pending cap ---------------------

func TestContentType415(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp := postJSON(t, ts.URL+"/api/entries", "text/plain", mustMarshal(t, validPayload()))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestBodyTooLarge413(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	payload := validPayload()
	payload["summary"] = strings.Repeat("a", 9000)
	resp := postJSON(t, ts.URL+"/api/entries", "application/json", mustMarshal(t, payload))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", resp.StatusCode)
	}
}

func TestRateLimit429(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	body := mustMarshal(t, validPayload())

	for i := 0; i < rateLimitPerHour; i++ {
		resp := postJSON(t, ts.URL+"/api/entries", "application/json", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("request %d: status = %d, want 201", i+1, resp.StatusCode)
		}
	}

	resp := postJSON(t, ts.URL+"/api/entries", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th request: status = %d, want 429", resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra == "" {
		t.Fatalf("6th request: missing Retry-After header")
	}
}

func TestRateLimitCountsMalformedPosts429(t *testing.T) {
	ts, _ := newTestServer(t, Config{})

	for i := 0; i < rateLimitPerHour; i++ {
		resp := postJSON(t, ts.URL+"/api/entries", "text/plain", []byte("not json"))
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnsupportedMediaType {
			t.Fatalf("malformed request %d: status = %d, want 415", i+1, resp.StatusCode)
		}
	}

	resp := postJSON(t, ts.URL+"/api/entries", "application/json", mustMarshal(t, validPayload()))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th request (valid, after malformed ones): status = %d, want 429", resp.StatusCode)
	}
	if ra := resp.Header.Get("Retry-After"); ra == "" {
		t.Fatalf("6th request: missing Retry-After header")
	}
}

func TestPendingCap503(t *testing.T) {
	ts, _ := newTestServer(t, Config{PendingCap: 2})
	body := mustMarshal(t, validPayload())

	for i := 0; i < 2; i++ {
		resp := postJSON(t, ts.URL+"/api/entries", "application/json", body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("request %d: status = %d, want 201", i+1, resp.StatusCode)
		}
	}

	resp := postJSON(t, ts.URL+"/api/entries", "application/json", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("3rd request: status = %d, want 503", resp.StatusCode)
	}
}

// --- admin guard ----------------------------------------------------------

func TestAdminGuardNoHeader403(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp, err := http.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminGuardCrossOrigin403(t *testing.T) {
	ts, db := newTestServer(t, Config{AdminUsers: []string{"matt"}})
	id, err := db.Create(store.NewEntry{Name: "A", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/entries/"+id+"/approve", nil)
	req.Header.Set("X-Authentik-Username", "matt")
	req.Header.Set("Origin", "http://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminGuardUserNotInAllowlist403(t *testing.T) {
	ts, _ := newTestServer(t, Config{AdminUsers: []string{"matthew"}})
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("X-Authentik-Username", "anyone")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminGuardEmptyAllowlistDeniesAll403(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/admin", nil)
	req.Header.Set("X-Authentik-Username", "matthew")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAdminGuardApproveSucceeds303(t *testing.T) {
	ts, db := newTestServer(t, Config{AdminUsers: []string{"matt"}})
	id, err := db.Create(store.NewEntry{Name: "A", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/admin/entries/"+id+"/approve", nil)
	req.Header.Set("X-Authentik-Username", "matt")
	req.Header.Set("Origin", ts.URL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/admin" {
		t.Fatalf("Location = %q, want /admin", loc)
	}

	e, err := db.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if e.Status != store.StatusApproved {
		t.Fatalf("status = %q, want approved", e.Status)
	}
}

// --- entry page, api list, badge ------------------------------------------

func TestEntryPage404ForPendingAndHidden(t *testing.T) {
	ts, db := newTestServer(t, Config{})

	pendingID, err := db.Create(store.NewEntry{Name: "P", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	hiddenID, err := db.Create(store.NewEntry{Name: "H", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := db.Hide(hiddenID); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	for _, id := range []string{pendingID, hiddenID} {
		resp, err := http.Get(ts.URL + "/e/" + id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("id %s: status = %d, want 404", id, resp.StatusCode)
		}
	}
}

func TestAPIListApprovedOnly(t *testing.T) {
	ts, db := newTestServer(t, Config{})

	pendingID, _ := db.Create(store.NewEntry{Name: "P", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	approvedID, _ := db.Create(store.NewEntry{Name: "A", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	hiddenID, _ := db.Create(store.NewEntry{Name: "H", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err := db.Approve(approvedID); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := db.Hide(hiddenID); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/entries")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var out struct {
		Entries []EntryView `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(out.Entries))
	}
	if out.Entries[0].ID != approvedID {
		t.Fatalf("entry id = %s, want %s", out.Entries[0].ID, approvedID)
	}
	for _, e := range out.Entries {
		if e.ID == pendingID || e.ID == hiddenID {
			t.Fatalf("api list leaked non approved entry %s", e.ID)
		}
	}
}

func TestBadge404ForHidden200ForPending(t *testing.T) {
	ts, db := newTestServer(t, Config{})

	pendingID, _ := db.Create(store.NewEntry{Name: "P", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	hiddenID, _ := db.Create(store.NewEntry{Name: "H", Summary: "s", Source: "public", AuditTier: "audit", AuditDate: "2026-01-01"})
	if err := db.Hide(hiddenID); err != nil {
		t.Fatalf("Hide: %v", err)
	}

	resp, err := http.Get(ts.URL + "/badge/" + pendingID + ".svg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pending badge status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/svg+xml") {
		t.Fatalf("content type = %q", ct)
	}

	resp, err = http.Get(ts.URL + "/badge/" + hiddenID + ".svg")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("hidden badge status = %d, want 404", resp.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestMethodMismatch405(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp, err := http.Post(ts.URL+"/wall", "text/plain", nil)
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}

func TestUnknownRoute404(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp, err := http.Get(ts.URL + "/no/such/route")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSecurityHeadersPresent(t *testing.T) {
	ts, _ := newTestServer(t, Config{})
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "Referrer-Policy", "X-Frame-Options"} {
		if resp.Header.Get(h) == "" {
			t.Fatalf("missing header %s", h)
		}
	}
}
