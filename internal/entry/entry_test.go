package entry

import (
	"strings"
	"testing"
	"time"
)

func strp(s string) *string { return &s }
func intp(i int) *int       { return &i }

// validRecord is the baseline every table case mutates.
func validRecord() Record {
	return Record{
		Name: strp("MSP Sentinel"), Summary: strp("did the closeout"),
		Source: strp("public"), AuditTier: strp("audit"), AuditDate: strp("2026-01-01"),
		Found: intp(10), Fixed: intp(10), Accepted: intp(0),
		CriticalOpen: intp(0), CriticalAccepted: intp(0),
	}
}

var testNow = time.Date(2026, 9, 11, 0, 0, 0, 0, time.UTC)

func TestValidateTable(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(Record) Record
		wantErr bool
	}{
		{"valid record is accepted", func(r Record) Record { return r }, false},
		{"missing name", func(r Record) Record { r.Name = nil; return r }, true},
		{"name too long", func(r Record) Record { r.Name = strp(strings.Repeat("a", 61)); return r }, true},
		{"name at the limit", func(r Record) Record { r.Name = strp(strings.Repeat("a", 60)); return r }, false},
		{"name empty after trim", func(r Record) Record { r.Name = strp("   "); return r }, true},
		{"name has control char", func(r Record) Record { r.Name = strp("bad\x01name"); return r }, true},
		{"missing summary", func(r Record) Record { r.Summary = nil; return r }, true},
		{"summary too long", func(r Record) Record { r.Summary = strp(strings.Repeat("a", 161)); return r }, true},
		{"missing source", func(r Record) Record { r.Source = nil; return r }, true},
		{"bad source value", func(r Record) Record { r.Source = strp("private"); return r }, true},
		{"repo url no host", func(r Record) Record { r.RepoURL = strp("https://"); return r }, true},
		{"repo url too long", func(r Record) Record {
			r.RepoURL = strp("https://example.com/" + strings.Repeat("a", 200))
			return r
		}, true},
		{"repo url set when source closed", func(r Record) Record {
			r.Source = strp("closed")
			r.RepoURL = strp("https://example.com/a")
			return r
		}, true},
		{"repo url ok when source public", func(r Record) Record {
			r.RepoURL = strp("https://example.com/a")
			return r
		}, false},
		{"missing audit tier", func(r Record) Record { r.AuditTier = nil; return r }, true},
		{"bad audit tier", func(r Record) Record { r.AuditTier = strp("weekly"); return r }, true},
		{"missing audit date", func(r Record) Record { r.AuditDate = nil; return r }, true},
		{"bad audit date format", func(r Record) Record { r.AuditDate = strp("09/01/2026"); return r }, true},
		{"impossible calendar date", func(r Record) Record { r.AuditDate = strp("2026-02-30"); return r }, true},
		{"audit date before floor", func(r Record) Record { r.AuditDate = strp("2024-12-31"); return r }, true},
		{"audit date after today", func(r Record) Record { r.AuditDate = strp("2026-09-12"); return r }, true},
		{"audit date today is ok", func(r Record) Record { r.AuditDate = strp("2026-09-11"); return r }, false},
		{"missing found", func(r Record) Record { r.Found = nil; return r }, true},
		{"missing fixed", func(r Record) Record { r.Fixed = nil; return r }, true},
		{"missing accepted", func(r Record) Record { r.Accepted = nil; return r }, true},
		{"missing critical open", func(r Record) Record { r.CriticalOpen = nil; return r }, true},
		{"missing critical accepted", func(r Record) Record { r.CriticalAccepted = nil; return r }, true},
		{"found too high", func(r Record) Record { r.Found = intp(10000); return r }, true},
		{"found negative", func(r Record) Record { r.Found = intp(-1); return r }, true},
		{"critical accepted negative", func(r Record) Record { r.CriticalAccepted = intp(-1); return r }, true},
		{"fixed plus accepted exceeds found", func(r Record) Record {
			r.Found, r.Fixed, r.Accepted = intp(10), intp(8), intp(5)
			return r
		}, true},
		{"critical open exceeds found", func(r Record) Record {
			r.Found, r.Fixed, r.CriticalOpen = intp(2), intp(0), intp(3)
			return r
		}, true},
		{"critical open plus critical accepted exceeds found", func(r Record) Record {
			r.Found, r.Fixed, r.CriticalOpen, r.CriticalAccepted = intp(4), intp(0), intp(3), intp(2)
			return r
		}, true},
		{"critical open plus critical accepted at found", func(r Record) Record {
			r.Found, r.Fixed, r.Accepted, r.CriticalOpen, r.CriticalAccepted = intp(4), intp(0), intp(2), intp(2), intp(2)
			return r
		}, false},
		{"critical accepted exceeds accepted", func(r Record) Record {
			r.Found, r.Fixed, r.Accepted, r.CriticalOpen, r.CriticalAccepted = intp(4), intp(0), intp(0), intp(2), intp(2)
			return r
		}, true},
		{"critical accepted at accepted", func(r Record) Record {
			r.Found, r.Fixed, r.Accepted, r.CriticalOpen, r.CriticalAccepted = intp(4), intp(0), intp(2), intp(0), intp(2)
			return r
		}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, msg := Validate(c.mutate(validRecord()), testNow)
			if c.wantErr && msg == "" {
				t.Fatalf("expected a validation error, got none")
			}
			if !c.wantErr && msg != "" {
				t.Fatalf("expected no error, got %q", msg)
			}
		})
	}
}

// TestValidateRejectsFormatCharacters is the B1 fix: invisible format and
// bidi runes let a stored name read as something else on the page.
func TestValidateRejectsFormatCharacters(t *testing.T) {
	hostile := []struct {
		name string
		s    string
	}{
		{"right to left override", "paypal" + string(rune(0x202E)) + " gro.live"},
		{"zero width space", "pay" + string(rune(0x200B)) + "pal"},
		{"zero width joiner", "pay" + string(rune(0x200D)) + "pal"},
		{"byte order mark", "paypal" + string(rune(0xFEFF))},
		{"bidi isolates", "paypal" + string(rune(0x2066)) + "x" + string(rune(0x2069))},
		{"left to right embedding", "pay" + string(rune(0x202A)) + "pal"},
		{"pop directional isolate", "paypal" + string(rune(0x2069))},
		{"right to left mark", "paypal" + string(rune(0x200F))},
		{"arabic letter mark", "paypal" + string(rune(0x061C))},
	}

	for _, h := range hostile {
		t.Run("name "+h.name, func(t *testing.T) {
			r := validRecord()
			r.Name = strp(h.s)
			if _, msg := Validate(r, testNow); msg == "" {
				t.Fatalf("name %q was accepted, want rejection", h.s)
			}
		})
		t.Run("summary "+h.name, func(t *testing.T) {
			r := validRecord()
			r.Summary = strp(h.s)
			if _, msg := Validate(r, testNow); msg == "" {
				t.Fatalf("summary %q was accepted, want rejection", h.s)
			}
		})
	}
}

// TestValidateAllowsLegitimateText guards the B1 fix against overreach:
// accents, combining marks, other scripts and emoji are all legal.
func TestValidateAllowsLegitimateText(t *testing.T) {
	good := []string{"café", "ma" + string(rune(0x0301)), "日本語ツール", "shipit 🚀", "Ünïcödé tool"}
	for _, s := range good {
		r := validRecord()
		r.Name = strp(s)
		if _, msg := Validate(r, testNow); msg != "" {
			t.Fatalf("name %q was rejected: %s", s, msg)
		}
	}
}

// TestValidateRepoURL is the B2 fix: userinfo in the authority makes a link
// read as one host and go to another.
func TestValidateRepoURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool // true means accepted
	}{
		{"https://github.com@evil.example.com/x", false},
		{"https://github.com:x@evil.example.com/x", false},
		{"https://user@example.com/a", false},
		{"javascript:alert(1)", false},
		{"data:text/html,hi", false},
		{"file:///etc/passwd", false},
		{"ftp://example.com/a", false},
		{"https://example.com/a#frag", false},
		{"https://example.com/a#", false},
		{"//example.com/a", false},
		{"https://exam" + string(rune(0x200B)) + "ple.com/a", false},
		{"https://github.com/a/b", true},
		{"https://gitlab.com/a/b", true},
		{"https://codeberg.org/a/b", true},
		{"https://git.parallax.example:3000/a/b", true},
		{"http://example.com/a", true},
		{"http://192.0.2.10:8080/a", true},
	}

	for _, c := range cases {
		t.Run(c.url, func(t *testing.T) {
			r := validRecord()
			r.RepoURL = strp(c.url)
			_, msg := Validate(r, testNow)
			if c.want && msg != "" {
				t.Fatalf("repo url %q was rejected: %s", c.url, msg)
			}
			if !c.want && msg == "" {
				t.Fatalf("repo url %q was accepted, want rejection", c.url)
			}
		})
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	cases := map[string]string{
		"unknown field": `{"name":"A","extra_field":"nope"}`,
		"an id inside":  `{"name":"A","id":"abcdefghijkl"}`,
		"two objects":   `{"name":"A"}{"name":"B"}`,
		"broken json":   `{"name":`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(body)); err == nil {
				t.Fatalf("decode accepted %s", body)
			}
		})
	}
}

func TestCriticalSumsOpenAndAccepted(t *testing.T) {
	e := Entry{CriticalOpen: 1, CriticalAccepted: 2}
	if got := e.Critical(); got != 3 {
		t.Fatalf("Critical() = %d, want 3", got)
	}
}
