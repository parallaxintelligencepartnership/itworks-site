// Package badge computes the color and renders the SVG for an itworks.dev
// audit badge. No external references are ever emitted in the SVG.
package badge

import (
	"fmt"
	"math"
	"time"
	"unicode/utf8"
)

const (
	ColorGreen = "green"
	ColorAmber = "amber"
	ColorRed   = "red"
)

// hex values for each badge color, plus the fixed label background and text.
const (
	hexGreen = "#3fb950"
	hexAmber = "#d29922"
	hexRed   = "#f85149"
	hexLabel = "#24292f"
	hexText  = "#ffffff"
)

const (
	badgeHeight   = 20
	charWidth     = 6.5
	segPadPerSide = 10.0
	fontFamily    = "Verdana,Geneva,DejaVu Sans,sans-serif"
	fontSize      = 11
	leftLabel     = "itworks.dev audit"
)

// Color returns "red", "amber", or "green" for the given audit date, open
// critical count, and current time. criticalOpen > 0 always wins (red). Else
// the badge is amber once the UTC calendar date gap from auditDate to now is
// 30 days or more, green below that.
func Color(auditDate time.Time, criticalOpen int, now time.Time) string {
	if criticalOpen > 0 {
		return ColorRed
	}
	a := time.Date(auditDate.UTC().Year(), auditDate.UTC().Month(), auditDate.UTC().Day(), 0, 0, 0, 0, time.UTC)
	n := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	days := int(math.Round(n.Sub(a).Hours() / 24))
	if days >= 30 {
		return ColorAmber
	}
	return ColorGreen
}

func hexFor(color string) string {
	switch color {
	case ColorRed:
		return hexRed
	case ColorAmber:
		return hexAmber
	default:
		return hexGreen
	}
}

func segWidth(s string) int {
	return int(math.Round(float64(utf8.RuneCountInString(s))*charWidth)) + int(segPadPerSide*2)
}

// Render returns a flat, shields style SVG badge. date is the YYYY-MM-DD
// audit date to display, criticalOpen the open critical count, and color one
// of "green", "amber", "red" (the right segment fill).
func Render(date string, criticalOpen int, color string) []byte {
	rightText := fmt.Sprintf("%s · %d critical open", date, criticalOpen)
	rightHex := hexFor(color)

	leftW := segWidth(leftLabel)
	rightW := segWidth(rightText)
	totalW := leftW + rightW

	leftCenter := leftW / 2
	rightCenter := leftW + rightW/2

	title := fmt.Sprintf("Audit %s, %d critical open, %s", date, criticalOpen, color)

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" role="img" aria-label="%s">`+
		`<title>%s</title>`+
		`<rect width="%d" height="%d" fill="%s"/>`+
		`<rect x="%d" width="%d" height="%d" fill="%s"/>`+
		`<g fill="%s" font-family="%s" font-size="%d" text-anchor="middle">`+
		`<text x="%d" y="14">%s</text>`+
		`<text x="%d" y="14">%s</text>`+
		`</g>`+
		`</svg>`,
		totalW, badgeHeight, title,
		title,
		leftW, badgeHeight, hexLabel,
		leftW, rightW, badgeHeight, rightHex,
		hexText, fontFamily, fontSize,
		leftCenter, leftLabel,
		rightCenter, rightText,
	)
	return []byte(svg)
}
