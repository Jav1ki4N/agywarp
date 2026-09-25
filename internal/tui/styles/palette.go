package styles

import (
	"fmt"
	"image/color"

	"charm.land/lipgloss/v2"
)

// Color Palette for TUI components and pages
var (
	// Neutral / Grayscale
	ColorDarkGray     = lipgloss.Color("240") // Subtle borders, delimiters
	ColorDimGray      = lipgloss.Color("243") // Secondary muted text
	ColorHintFg       = lipgloss.Color("247") // Command hints (clean light gray, lighter than bar)
	ColorLightGray    = lipgloss.Color("250") // Standard text, labels
	ColorBarBg        = lipgloss.Color("236") // Fallback bar background (~#303030)
	ColorWhite        = lipgloss.Color("255") // High-contrast text
	ColorStatusDarkBg = lipgloss.Color("238") // Dark background for status label block (~#444444)

	// Accents & Semantics
	ColorPrimary = lipgloss.Color("63")  // Antigravity / Charm purple
	ColorSuccess = lipgloss.Color("42")  // Connected / Active green
	ColorWarning = lipgloss.Color("214") // In-progress / Warning yellow
	ColorDanger  = lipgloss.Color("196") // Error / Offline / Disconnected red
)

// ElevateColor dynamically computes an adjusted brightness color based on
// a reference base color and an offset delta.
func ElevateColor(c color.Color, delta int) color.Color {
	if c == nil {
		return ColorBarBg
	}
	r32, g32, b32, _ := c.RGBA()
	r := int(r32 >> 8)
	g := int(g32 >> 8)
	b := int(b32 >> 8)

	// Perceived luminance formula (ITU-R BT.601)
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)

	var newR, newG, newB int
	if lum < 128 { // Dark background: raise brightness
		newR = min(r+delta, 255)
		newG = min(g+delta, 255)
		newB = min(b+delta, 255)
	} else { // Light background: lower brightness
		newR = max(r-delta, 0)
		newG = max(g-delta, 0)
		newB = max(b-delta, 0)
	}

	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", newR, newG, newB))
}
