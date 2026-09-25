package components

import (
	"image/color"
	"strings"
	"testing"

	"agywarp/internal/traffic"

	"charm.land/lipgloss/v2"
)

func TestTrafficChartRenderAndHeight(t *testing.T) {
	chart := NewTrafficChart()
	chart.Width = 60
	chart.Height = 8

	// Add mock samples
	chart.AddSample(traffic.Sample{
		DownSpeed:     102400, // 100 KB/s
		UpSpeed:       51200,  // 50 KB/s
		TotalSpeed:    153600,
		InterfaceName: "wlp3s0",
		IsProxy:       false,
	})

	output := chart.Render()
	lines := strings.Split(output, "\n")

	if len(lines) != chart.Height {
		t.Errorf("expected %d lines for TrafficChart, got %d", chart.Height, len(lines))
	}

	// Verify header contains interface name and formatted speed
	header := lines[0]
	if !strings.Contains(header, "wlp3s0") {
		t.Errorf("expected header to contain 'wlp3s0', got: %s", header)
	}
	if !strings.Contains(header, "Default") {
		t.Errorf("expected header to contain 'Default', got: %s", header)
	}

	// Verify columns contain block characters
	foundBlock := false
	for _, l := range lines[1:] {
		for _, r := range blockRunes[1:] {
			if strings.ContainsRune(l, r) {
				foundBlock = true
				break
			}
		}
		if foundBlock {
			break
		}
	}
	if !foundBlock {
		t.Errorf("expected rendered traffic chart to contain block characters, got:\n%s", output)
	}
}

func TestTrafficChartLowSpeedNotVoid(t *testing.T) {
	chart := NewTrafficChart()
	chart.Width = 40
	chart.Height = 7

	// Add a huge spike first to inflate peak
	chart.AddSample(traffic.Sample{
		TotalSpeed: 10 * 1024 * 1024, // 10 MB/s
	})

	// Add a tiny speed (e.g. 50 B/s - slow ping)
	chart.AddSample(traffic.Sample{
		TotalSpeed: 50, // 50 B/s
	})

	output := chart.Render()

	// Verify that the chart contains block runes and is NOT a complete void
	foundBlock := false
	for _, r := range blockRunes[1:] {
		if strings.ContainsRune(output, r) {
			foundBlock = true
			break
		}
	}
	if !foundBlock {
		t.Fatalf("expected 50 B/s traffic to render visible block runes instead of empty void, got:\n%s", output)
	}
}

func TestTrafficChartZeroSpeedHasFloorBaseline(t *testing.T) {
	chart := NewTrafficChart()
	chart.Width = 30
	chart.Height = 6

	// Clean chart with 0 speed
	output := chart.Render()

	// Should contain the subtle floor baseline rune '·'
	if !strings.ContainsRune(output, '·') {
		t.Fatalf("expected idle chart to render floor baseline dots '·', got:\n%s", output)
	}
}

func TestTrafficChartGradientGeneration(t *testing.T) {
	// Test 1: Dark background (e.g. #141414, lum ~20)
	darkBg := lipgloss.Color("#141414")
	gradDark := generateVerticalGradient(6, darkBg)
	if len(gradDark) != 6 {
		t.Fatalf("expected 6 gradient colors, got %d", len(gradDark))
	}

	rTop, gTop, bTop, _ := gradDark[0].RGBA()
	rBot, gBot, bBot, _ := gradDark[5].RGBA()
	rBg, gBg, bBg, _ := darkBg.RGBA()

	lumTop := 0.299*float64(rTop) + 0.587*float64(gTop) + 0.114*float64(bTop)
	lumBot := 0.299*float64(rBot) + 0.587*float64(gBot) + 0.114*float64(bBot)
	lumBg := 0.299*float64(rBg) + 0.587*float64(gBg) + 0.114*float64(bBg)

	// In dark mode:
	// 1. Bottom color MUST be brighter than background
	if lumBot <= lumBg {
		t.Errorf("expected bottom color (lum=%f) to be brighter than dark background (lum=%f)", lumBot, lumBg)
	}
	// 2. Top color MUST be brighter than bottom color (gain brightness as speed rises)
	if lumTop <= lumBot {
		t.Errorf("expected top color (lum=%f) to be brighter than bottom color (lum=%f)", lumTop, lumBot)
	}

	// Test 2: Light background (e.g. #f0f0f0, lum ~240)
	lightBg := color.RGBA{R: 240, G: 240, B: 240, A: 255}
	gradLight := generateVerticalGradient(6, lightBg)
	rLightBot, gLightBot, bLightBot, _ := gradLight[5].RGBA()
	rLightBg, gLightBg, bLightBg, _ := lightBg.RGBA()

	lumLightBot := 0.299*float64(rLightBot) + 0.587*float64(gLightBot) + 0.114*float64(bLightBot)
	lumLightBg := 0.299*float64(rLightBg) + 0.587*float64(gLightBg) + 0.114*float64(bLightBg)

	// In light mode:
	// Bottom color MUST be darker than background
	if lumLightBot >= lumLightBg {
		t.Errorf("expected bottom color (lum=%f) to be darker than light background (lum=%f)", lumLightBot, lumLightBg)
	}
}

func TestFormatSpeed(t *testing.T) {
	tests := []struct {
		bytes    float64
		expected string
	}{
		{500, "500 B/s"},
		{1024, "1.0 KB/s"},
		{153600, "150.0 KB/s"},
		{5242880, "5.0 MB/s"},
		{1073741824, "1.00 GB/s"},
	}

	for _, tt := range tests {
		got := FormatSpeed(tt.bytes)
		if got != tt.expected {
			t.Errorf("FormatSpeed(%f) = %s, expected %s", tt.bytes, got, tt.expected)
		}
	}
}
