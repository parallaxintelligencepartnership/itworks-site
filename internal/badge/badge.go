// Package badge computes the color and renders the SVG for an itworks.build
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

// hex values for each badge color, plus the fixed label plate and text
// colors.
const (
	hexGreen    = "#1a7f37"
	hexAmber    = "#d29922"
	hexRed      = "#b3261e"
	hexLabel    = "#1f1f1f"
	hexText     = "#ffffff"
	hexAmberInk = "#1f1f1f"
	hexHairline = "#000000"
)

const (
	badgeHeight       = 20
	charWidth         = 6.6
	labelPad          = 10.0
	fontFamily        = "-apple-system,BlinkMacSystemFont,Segoe UI,Helvetica,Arial,sans-serif"
	fontSize          = 11
	fontWeight        = 600
	leftLabel         = "itworks.build"
	glyphCenterOffset = 15.0 // from left plate edge (leftW) to glyph cx
	textStartOffset   = 32.0 // from left plate edge (leftW) to right-text x
	trailingPad       = 12.0 // trailing space after right text
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

// textHexFor returns the right-segment text color for the given badge color.
// Amber uses dark ink text; green and red use white.
func textHexFor(color string) string {
	if color == ColorAmber {
		return hexAmberInk
	}
	return hexText
}

// textLen approximates the rendered width of s at 11px semibold in the
// badge's system font stack, about 6.6px per character.
func textLen(s string) float64 {
	return math.Round(float64(utf8.RuneCountInString(s)) * charWidth)
}

// glyph returns the SVG markup for the state glyph (disc, diamond, or
// triangle) centered at (cx, cy=10), plus a data-shape attribute identifying
// the shape for tests, and the fill color to use.
func glyph(color string, cx float64) string {
	switch color {
	case ColorAmber:
		// Diamond: half-diagonal 5.6, ink fill.
		return fmt.Sprintf(`<path data-shape="diamond" d="M%.1f 4.4 %.1f 10 %.1f 15.6 %.1f 10Z" fill="%s"/>`,
			cx, cx+5.6, cx, cx-5.6, hexAmberInk)
	case ColorRed:
		// Triangle, white fill.
		return fmt.Sprintf(`<path data-shape="triangle" d="M%.1f 3.5 %.1f 14.4 %.1f 14.4Z" fill="%s"/>`,
			cx, cx+5.8, cx-5.8, hexText)
	default:
		// Green: filled disc r 4.6, white fill.
		return fmt.Sprintf(`<circle data-shape="disc" cx="%.1f" cy="10" r="4.6" fill="%s"/>`, cx, hexText)
	}
}

// Render returns the Ledger style SVG badge (design source:
// docs/design/ledger-mock-v2.html). date is the YYYY-MM-DD audit date to
// display, criticalOpen the open critical count, and color one of "green",
// "amber", "red" (the state plate fill).
func Render(date string, criticalOpen int, color string) []byte {
	var rightText, title string
	if color == ColorAmber {
		rightText = fmt.Sprintf("%s · %d critical · stale", date, criticalOpen)
		title = fmt.Sprintf("Audit %s, stale, %d critical, %s", date, criticalOpen, color)
	} else {
		rightText = fmt.Sprintf("%s · %d critical", date, criticalOpen)
		title = fmt.Sprintf("Audit %s, %d critical, %s", date, criticalOpen, color)
	}
	stateHex := hexFor(color)
	rightTextHex := textHexFor(color)

	labelLen := textLen(leftLabel)
	leftW := int(math.Round(labelLen + labelPad*2))

	rightLen := textLen(rightText)
	rightW := int(math.Round(textStartOffset + rightLen + trailingPad))

	totalW := leftW + rightW

	labelCenter := float64(leftW) / 2
	glyphCX := float64(leftW) + glyphCenterOffset
	textX := float64(leftW) + textStartOffset

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %[1]d %[2]d" role="img" aria-labelledby="itworks-badge-title">`+
		`<title id="itworks-badge-title">%s</title>`+
		`<g shape-rendering="crispEdges">`+
		`<rect width="%d" height="%d" fill="%s"/>`+
		`<rect width="%d" height="%d" fill="%s"/>`+
		`<rect x="%d" width="1" height="%d" fill="%s" opacity=".22"/>`+
		`</g>`+
		`%s`+
		`<g font-family="%s" font-size="%d" font-weight="%d">`+
		`<text x="%.1f" y="14.2" fill="%s" text-anchor="middle" textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`+
		`<text x="%.1f" y="14.2" fill="%s" textLength="%.1f" lengthAdjust="spacingAndGlyphs">%s</text>`+
		`</g>`+
		`</svg>`,
		totalW, badgeHeight,
		title,
		totalW, badgeHeight, stateHex,
		leftW, badgeHeight, hexLabel,
		leftW, badgeHeight, hexHairline,
		glyph(color, glyphCX),
		fontFamily, fontSize, fontWeight,
		labelCenter, hexText, labelLen, leftLabel,
		textX, rightTextHex, rightLen, rightText,
	)
	return []byte(svg)
}
