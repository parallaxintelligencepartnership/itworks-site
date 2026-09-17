package badge

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestKickoffHarnessProof(t *testing.T) {
	// A same day, no critical audit is green.
	got := Color(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), 0, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if got != ColorGreen {
		t.Fatalf("kickoff proof: expected %q, got %q", ColorGreen, got)
	}
}

func TestColorBoundaries(t *testing.T) {
	audit := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		now          time.Time
		criticalOpen int
		want         string
	}{
		{"29 days is green", audit.AddDate(0, 0, 29), 0, ColorGreen},
		{"30 days is amber", audit.AddDate(0, 0, 30), 0, ColorAmber},
		{"critical open beats fresh audit", audit, 1, ColorRed},
		{"red beats amber", audit.AddDate(0, 0, 40), 1, ColorRed},
		{"same day is green", audit, 0, ColorGreen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Color(audit, c.criticalOpen, c.now)
			if got != c.want {
				t.Fatalf("Color() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRenderContainsExpectedContent(t *testing.T) {
	svg := string(Render("2026-09-07", 3, ColorRed))

	if !strings.Contains(svg, "2026-09-07") {
		t.Errorf("render missing date: %s", svg)
	}
	if !strings.Contains(svg, "3 critical") {
		t.Errorf("render missing critical count: %s", svg)
	}
	if !strings.Contains(svg, hexRed) {
		t.Errorf("render missing red hex: %s", svg)
	}
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "</svg>") {
		t.Errorf("render is not a well formed svg tag: %s", svg)
	}
	if strings.Contains(svg, "http://") && !strings.Contains(svg, "www.w3.org") {
		t.Errorf("render appears to reference an external resource: %s", svg)
	}
}

func TestRenderContainsLabel(t *testing.T) {
	for _, color := range []string{ColorGreen, ColorAmber, ColorRed} {
		svg := string(Render("2026-01-01", 1, color))
		if !strings.Contains(svg, "itworks.dev") {
			t.Errorf("%s render missing label 'itworks.dev': %s", color, svg)
		}
	}
}

func TestRenderGlyphShapePerState(t *testing.T) {
	green := string(Render("2026-01-01", 0, ColorGreen))
	amber := string(Render("2026-01-01", 0, ColorAmber))
	red := string(Render("2026-01-01", 1, ColorRed))

	if !strings.Contains(green, `data-shape="disc"`) {
		t.Errorf("green render missing disc glyph: %s", green)
	}
	if !strings.Contains(amber, `data-shape="diamond"`) {
		t.Errorf("amber render missing diamond glyph: %s", amber)
	}
	if !strings.Contains(red, `data-shape="triangle"`) {
		t.Errorf("red render missing triangle glyph: %s", red)
	}
}

func TestRenderGreenAndAmberHex(t *testing.T) {
	if !strings.Contains(string(Render("2026-01-01", 0, ColorGreen)), hexGreen) {
		t.Errorf("green render missing green hex")
	}
	if !strings.Contains(string(Render("2026-01-01", 0, ColorAmber)), hexAmber) {
		t.Errorf("amber render missing amber hex")
	}
}

func TestRenderAmberIsStaleWithInkText(t *testing.T) {
	svg := string(Render("2026-01-01", 2, ColorAmber))
	if !strings.Contains(svg, "stale") {
		t.Errorf("amber render missing 'stale': %s", svg)
	}
	if !strings.Contains(svg, hexAmberInk) {
		t.Errorf("amber render missing ink text hex %s: %s", hexAmberInk, svg)
	}
	if !strings.Contains(svg, "Audit 2026-01-01, stale, 2 critical, amber") {
		t.Errorf("amber render missing expected title: %s", svg)
	}
}

func TestRenderGreenAndRedDoNotContainStale(t *testing.T) {
	green := string(Render("2026-01-01", 0, ColorGreen))
	red := string(Render("2026-01-01", 1, ColorRed))
	if strings.Contains(green, "stale") {
		t.Errorf("green render should not contain 'stale': %s", green)
	}
	if strings.Contains(red, "stale") {
		t.Errorf("red render should not contain 'stale': %s", red)
	}
}

func TestRenderNeverContainsOldHexValues(t *testing.T) {
	for _, color := range []string{ColorGreen, ColorAmber, ColorRed} {
		svg := string(Render("2026-01-01", 1, color))
		if strings.Contains(svg, "#3fb950") {
			t.Errorf("%s render still contains old green hex #3fb950: %s", color, svg)
		}
		if strings.Contains(svg, "#f85149") {
			t.Errorf("%s render still contains old red hex #f85149: %s", color, svg)
		}
	}
}

func TestRenderColorHexesAreExclusiveToTheirState(t *testing.T) {
	green := string(Render("2026-01-01", 0, ColorGreen))
	amber := string(Render("2026-01-01", 0, ColorAmber))
	red := string(Render("2026-01-01", 1, ColorRed))

	cases := []struct {
		name string
		svg  string
		hex  string
	}{
		{"green", green, hexGreen},
		{"amber", amber, hexAmber},
		{"red", red, hexRed},
	}
	others := map[string][]string{
		"green": {amber, red},
		"amber": {green, red},
		"red":   {green, amber},
	}
	for _, c := range cases {
		if !strings.Contains(c.svg, c.hex) {
			t.Errorf("%s render missing its own hex %s", c.name, c.hex)
		}
		for _, o := range others[c.name] {
			if strings.Contains(o, c.hex) {
				t.Errorf("%s hex %s leaked into another state's render", c.name, c.hex)
			}
		}
	}
}

func TestRenderNeverSaysCriticalOpen(t *testing.T) {
	for _, color := range []string{ColorGreen, ColorAmber, ColorRed} {
		svg := string(Render("2026-01-01", 1, color))
		if strings.Contains(svg, "critical open") {
			t.Errorf("%s render contains 'critical open': %s", color, svg)
		}
		titleStart := strings.Index(svg, "<title")
		titleEnd := strings.Index(svg, "</title>")
		if titleStart < 0 || titleEnd < 0 {
			t.Fatalf("%s render missing <title>: %s", color, svg)
		}
		if !strings.Contains(svg[titleStart:titleEnd], "1 critical") {
			t.Errorf("%s render title missing '1 critical': %s", color, svg)
		}
	}
}

func TestAmberWiderThanGreenForSameDateAndCount(t *testing.T) {
	green := string(Render("2026-01-01", 3, ColorGreen))
	amber := string(Render("2026-01-01", 3, ColorAmber))

	greenWidth := extractSVGWidth(t, green)
	amberWidth := extractSVGWidth(t, amber)

	if amberWidth <= greenWidth {
		t.Errorf("expected amber width (%d) > green width (%d)", amberWidth, greenWidth)
	}
}

// extractSVGWidth pulls the width="N" attribute value from the outer <svg> tag.
func extractSVGWidth(t *testing.T, svg string) int {
	t.Helper()
	const marker = `width="`
	i := strings.Index(svg, marker)
	if i < 0 {
		t.Fatalf("no width attribute found in svg: %s", svg)
	}
	rest := svg[i+len(marker):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		t.Fatalf("malformed width attribute in svg: %s", svg)
	}
	var width int
	if _, err := fmt.Sscanf(rest[:end], "%d", &width); err != nil {
		t.Fatalf("failed to parse width %q: %v", rest[:end], err)
	}
	return width
}
