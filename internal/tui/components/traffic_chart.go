package components

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"agywarp/internal/traffic"
	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

var blockRunes = []rune{' ', ' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// TrafficChart displays real-time network traffic columns with a background-aware vertical gradient.
type TrafficChart struct {
	ComponentBase
	Title string

	// Metrics
	InterfaceName string
	IsProxy       bool
	CurrentDown   float64
	CurrentUp     float64
	PeakSpeed     float64
	smoothedSpeed float64

	// History buffer of smoothed throughput data (bytes/second)
	History []float64

	// Dynamic colors
	BgColor  color.Color
	DelimFg  color.Color
	TitleFg  color.Color
	LabelFg  color.Color
	DimValFg color.Color
	TextFg   color.Color
}

// NewTrafficChart constructs a new TrafficChart component.
func NewTrafficChart() TrafficChart {
	return TrafficChart{
		ComponentBase: ComponentBase{Height: 8},
		Title:         "Traffic Monitor",
		InterfaceName: "detecting...",
		IsProxy:       false,
		PeakSpeed:     1024, // 1 KB/s baseline minimum for agile low-speed scaling
		History:       make([]float64, 0, 128),
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		LabelFg:       styles.ColorDimGray,
		DimValFg:      styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
}

func (t *TrafficChart) Init() tea.Cmd {
	return nil
}

func (t *TrafficChart) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		t.BgColor = msg
		t.DelimFg = styles.ElevateColor(msg, 35)
		t.TitleFg = styles.ElevateColor(msg, 75)
		t.LabelFg = styles.ElevateColor(msg, 65)
		t.DimValFg = styles.ElevateColor(msg, 55)
		t.TextFg = styles.ElevateColor(msg, 130)
	}
	return nil
}

// AddSample appends a new traffic sample to history with exponential moving average (EMA)
// smoothing and windowed dynamic peak tracking.
func (t *TrafficChart) AddSample(s traffic.Sample) {
	t.InterfaceName = s.InterfaceName
	t.IsProxy = s.IsProxy
	t.CurrentDown = s.DownSpeed
	t.CurrentUp = s.UpSpeed

	total := s.TotalSpeed

	// Exponential Moving Average (EMA) smoothing (alpha = 0.65)
	// Smooths bursty network spikes into a cohesive, fluid contour
	alpha := 0.65
	if len(t.History) == 0 {
		t.smoothedSpeed = total
	} else {
		t.smoothedSpeed = alpha*total + (1.0-alpha)*t.smoothedSpeed
	}

	t.History = append(t.History, t.smoothedSpeed)
	if len(t.History) > 256 {
		t.History = t.History[len(t.History)-256:]
	}

	// Dynamic peak tracking over the recent visible window (last 60 samples):
	// Allows the chart scale to naturally breathe down when high traffic stops
	windowMax := 1024.0 // 1 KB/s floor
	start := len(t.History) - 60
	if start < 0 {
		start = 0
	}
	for _, val := range t.History[start:] {
		if val > windowMax {
			windowMax = val
		}
	}

	if t.PeakSpeed > windowMax {
		// Smoothly decay peak towards windowMax so small traffic expands vertically
		t.PeakSpeed = t.PeakSpeed*0.92 + windowMax*0.08
	} else {
		t.PeakSpeed = windowMax
	}
}

// FormatSpeed returns a compact, human-readable speed string (e.g. "128.5 KB/s").
func FormatSpeed(bytesPerSec float64) string {
	if bytesPerSec < 0 {
		bytesPerSec = 0
	}
	if bytesPerSec < 1024 {
		return fmt.Sprintf("%.0f B/s", bytesPerSec)
	} else if bytesPerSec < 1024*1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	} else if bytesPerSec < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB/s", bytesPerSec/(1024*1024*1024))
}

// generateVerticalGradient creates a slice of colors from top (brightest peak) to bottom (base).
// The base color is calculated dynamically relative to the terminal background:
// - On a dark background, base is elevated (+35~+45) and cool-tinted so it is visibly brighter than the background.
// - On a light background, base is lowered (-40) so it is visibly darker than the background.
// - As rows move upwards towards row 0, luminance increases smoothly to peak highlight.
func generateVerticalGradient(steps int, bg color.Color) []color.Color {
	if steps <= 0 {
		return nil
	}

	var bgR, bgG, bgB int
	if bg != nil {
		r32, g32, b32, _ := bg.RGBA()
		bgR = int(r32 >> 8)
		bgG = int(g32 >> 8)
		bgB = int(b32 >> 8)
	} else {
		// Default dark background fallback (~#141414)
		bgR, bgG, bgB = 20, 20, 20
	}

	lum := 0.299*float64(bgR) + 0.587*float64(bgG) + 0.114*float64(bgB)
	isDark := (lum < 128)

	type rgb struct{ r, g, b float64 }
	var base, mid, peak rgb

	if isDark {
		// Dark background:
		// Base (Row steps-1): Distinctly brighter than background with cool indigo tint (+32, +38, +68)
		base = rgb{
			r: float64(min(bgR+32, 255)),
			g: float64(min(bgG+38, 255)),
			b: float64(min(bgB+68, 255)),
		}
		// Mid: Electric Indigo
		mid = rgb{r: 99, g: 102, b: 241} // #6366f1
		// Peak (Row 0): Radiant Bright Cyan (high luminance ~215)
		peak = rgb{r: 103, g: 232, b: 249} // #67e8f9
	} else {
		// Light background:
		// Base (Row steps-1): Distinctly darker than background (-45, -45, -25)
		base = rgb{
			r: float64(max(bgR-45, 0)),
			g: float64(max(bgG-45, 0)),
			b: float64(max(bgB-25, 0)),
		}
		mid = rgb{r: 79, g: 70, b: 229}
		peak = rgb{r: 14, g: 116, b: 144}
	}

	if steps == 1 {
		return []color.Color{lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(peak.r), int(peak.g), int(peak.b)))}
	}

	colors := make([]color.Color, steps)
	for r := 0; r < steps; r++ {
		// r = 0 is top (peak), r = steps-1 is bottom (base)
		t := float64(steps-1-r) / float64(steps-1) // 0.0 at bottom, 1.0 at top
		var cr, cg, cb float64
		if t <= 0.5 {
			factor := t * 2.0
			cr = base.r + factor*(mid.r-base.r)
			cg = base.g + factor*(mid.g-base.g)
			cb = base.b + factor*(mid.b-base.b)
		} else {
			factor := (t - 0.5) * 2.0
			cr = mid.r + factor*(peak.r-mid.r)
			cg = mid.g + factor*(peak.g-mid.g)
			cb = mid.b + factor*(peak.b-mid.b)
		}
		colors[r] = lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", int(cr), int(cg), int(cb)))
	}
	return colors
}

// Render generates the complete Traffic Chart view.
func (t *TrafficChart) Render() string {
	if t.Width <= 0 || t.Height <= 0 {
		return ""
	}

	delimFg := t.DelimFg
	if delimFg == nil {
		delimFg = styles.ColorDarkGray
	}
	titleFg := t.TitleFg
	if titleFg == nil {
		titleFg = styles.ColorDimGray
	}
	dimValFg := t.DimValFg
	if dimValFg == nil {
		dimValFg = styles.ColorDimGray
	}

	// 1. Header Line
	var titleStyle, delimStyle lipgloss.Style
	if t.Focused {
		titleStyle = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary)
		delimStyle = lipgloss.NewStyle().Foreground(styles.ColorDimGray)
	} else {
		titleStyle = lipgloss.NewStyle().Bold(false).Foreground(titleFg)
		delimStyle = lipgloss.NewStyle().Foreground(delimFg)
	}

	// Interface badge
	badgeType := "Default"
	if t.IsProxy {
		badgeType = "Proxy"
	}
	ifaceBadge := fmt.Sprintf("[%s: %s]", badgeType, t.InterfaceName)
	titlePrefix := fmt.Sprintf("── %s %s ", t.Title, ifaceBadge)

	// Speed readouts
	downStr := fmt.Sprintf("▼ %s", FormatSpeed(t.CurrentDown))
	upStr := fmt.Sprintf("▲ %s", FormatSpeed(t.CurrentUp))
	peakStr := fmt.Sprintf("Peak: %s", FormatSpeed(t.PeakSpeed))

	downStyle := lipgloss.NewStyle().Foreground(styles.ColorSuccess) // Green for download
	upStyle := lipgloss.NewStyle().Foreground(styles.ColorWarning)   // Amber for upload
	peakStyle := lipgloss.NewStyle().Foreground(dimValFg)

	readouts := fmt.Sprintf(" %s  %s  %s ──", downStyle.Render(downStr), upStyle.Render(upStr), peakStyle.Render(peakStr))
	prefixWidth := lipgloss.Width(titlePrefix)
	readoutsWidth := lipgloss.Width(readouts)

	var headerLine string
	if t.Width > prefixWidth+readoutsWidth {
		dashes := strings.Repeat("─", t.Width-(prefixWidth+readoutsWidth))
		headerLine = titleStyle.Render(titlePrefix) + delimStyle.Render(dashes) + readouts
	} else if t.Width > prefixWidth {
		headerLine = titleStyle.Render(titlePrefix) + delimStyle.Render(strings.Repeat("─", t.Width-prefixWidth))
	} else {
		headerLine = delimStyle.Render(strings.Repeat("─", t.Width))
	}

	if t.Height <= 1 {
		return headerLine
	}

	// 2. Determine Chart Area Dimensions
	hasAxis := (t.Height >= 5)
	axisRows := 0
	if hasAxis {
		axisRows = 1
	}

	plotHeight := t.Height - 1 - axisRows // Subtract header and optional axis
	if plotHeight < 1 {
		plotHeight = 1
	}

	plotWidth := t.Width - 2 // 1 column margin on left and right
	if plotWidth < 1 {
		plotWidth = 1
	}

	// 3. Prepare column data for the available plotWidth
	dataCount := len(t.History)
	columnData := make([]float64, plotWidth)
	startIndex := dataCount - plotWidth
	for x := 0; x < plotWidth; x++ {
		idx := startIndex + x
		if idx >= 0 && idx < dataCount {
			columnData[x] = t.History[idx]
		} else {
			columnData[x] = 0
		}
	}

	// 4. Compute column unit heights using power-law scaling (gamma = 0.45)
	// Total units per column = plotHeight * 8
	totalUnits := plotHeight * 8
	peak := t.PeakSpeed
	if peak <= 0 {
		peak = 1024
	}

	units := make([]int, plotWidth)
	for x := 0; x < plotWidth; x++ {
		val := columnData[x]
		if val <= 0 {
			units[x] = 0
			continue
		}
		ratio := val / peak
		if ratio > 1.0 {
			ratio = 1.0
		}
		// Non-linear power scaling lifts low-speed traffic out of the void,
		// ensuring the darkest base blocks are visibly rendered even for small packets.
		scaledRatio := math.Pow(ratio, 0.45)
		u := int(scaledRatio * float64(totalUnits))
		if u < 1 {
			u = 1 // Guaranteed minimum unit for active network traffic: NEVER void!
		} else if u > totalUnits {
			u = totalUnits
		}
		units[x] = u
	}

	// 5. Render Chart Plot with Vertical Gradient
	// row 0 is top (brightest), row plotHeight-1 is bottom (base)
	gradient := generateVerticalGradient(plotHeight, t.BgColor)
	faintFloorColor := styles.ElevateColor(t.BgColor, 15)

	var plotLines []string

	for r := 0; r < plotHeight; r++ {
		rowFloor := (plotHeight - 1 - r) * 8
		rowCeil := (plotHeight - r) * 8

		var rowRunes []rune
		var rowColors []color.Color

		for x := 0; x < plotWidth; x++ {
			u := units[x]
			if u >= rowCeil {
				rowRunes = append(rowRunes, '█')
				rowColors = append(rowColors, gradient[r])
			} else if u <= rowFloor {
				// If bottom row and column is idle (0 speed), render subtle floor baseline guide '·'
				if r == plotHeight-1 && u <= 0 {
					rowRunes = append(rowRunes, '·')
					rowColors = append(rowColors, faintFloorColor)
				} else {
					rowRunes = append(rowRunes, ' ')
					rowColors = append(rowColors, nil)
				}
			} else {
				sub := u - rowFloor
				if sub >= 1 && sub <= 7 {
					rowRunes = append(rowRunes, blockRunes[sub])
					rowColors = append(rowColors, gradient[r])
				} else {
					rowRunes = append(rowRunes, ' ')
					rowColors = append(rowColors, nil)
				}
			}
		}

		// Group consecutive runes of identical color into chunks for efficient rendering
		var b strings.Builder
		b.WriteString(" ") // left padding margin
		for i := 0; i < len(rowRunes); {
			c := rowColors[i]
			j := i + 1
			for j < len(rowRunes) && rowColors[j] == c {
				j++
			}
			chunk := string(rowRunes[i:j])
			if c != nil {
				b.WriteString(lipgloss.NewStyle().Foreground(c).Render(chunk))
			} else {
				b.WriteString(chunk)
			}
			i = j
		}
		b.WriteString(" ") // right padding margin
		plotLines = append(plotLines, b.String())
	}

	// 6. Optional Time / Scale Axis
	var axisLine string
	if hasAxis {
		axisDelim := lipgloss.NewStyle().Foreground(delimFg)
		dimText := lipgloss.NewStyle().Foreground(dimValFg)

		timeInfo := fmt.Sprintf("-%ds", plotWidth/2)
		axisLeft := dimText.Render("0 B/s")
		axisMid := axisDelim.Render(strings.Repeat("·", max(0, (plotWidth/2)-lipgloss.Width(timeInfo)-6))) + " " + dimText.Render(timeInfo) + " "
		axisRight := dimText.Render("now")

		spaceNeeded := plotWidth - lipgloss.Width(axisLeft) - lipgloss.Width(axisMid) - lipgloss.Width(axisRight)
		if spaceNeeded < 0 {
			spaceNeeded = 0
		}
		axisContent := axisLeft + axisMid + axisDelim.Render(strings.Repeat("·", spaceNeeded)) + axisRight
		axisLine = " " + axisContent + " "
	}

	// Assemble lines
	var lines []string
	lines = append(lines, headerLine)
	lines = append(lines, plotLines...)
	if hasAxis {
		lines = append(lines, axisLine)
	}

	return strings.Join(lines, "\n")
}
