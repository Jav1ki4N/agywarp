package components

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFooterOverflowKeepsHintsDuringRefresh(t *testing.T) {
	f := NewFooter()
	f.Hints = []FooterHint{Hint("tab: next block"), Hint("space: start"), Hint("t: test node"), Hint("p: proxy mode"), Hint("r: refresh"), Hint("q: quit")}
	for _, state := range []string{"DOWN", "UP", "REFRESHING"} {
		f.State = state
		for _, width := range []int{1, 20, 65, 80, 120} {
			f.Width = width
			rendered := f.Render()
			if lipgloss.Width(rendered) != width || strings.Contains(rendered, "\n") {
				t.Fatalf("state %s width %d: %q", state, width, rendered)
			}
			plain := ansi.Strip(rendered)
			if width >= 65 && (!strings.Contains(plain, "tab: next block") || !strings.Contains(plain, state)) {
				t.Fatalf("hints or state disappeared: %q", plain)
			}
		}
	}
}

func TestFooterBackgroundPreservesTerminalHue(t *testing.T) {
	f := NewFooter()
	if f.BarBg != nil {
		t.Fatal("unknown terminal background must remain transparent")
	}
	f.Update(tea.BackgroundColorMsg{Color: color.RGBA{R: 40, G: 60, B: 80, A: 255}})
	r, g, b, _ := f.BarBg.RGBA()
	if r>>8 != 34 || g>>8 != 51 || b>>8 != 68 {
		t.Fatalf("background = %d %d %d", r>>8, g>>8, b>>8)
	}
}
