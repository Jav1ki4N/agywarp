package components

import (
	"fmt"
	"testing"

	"agywarp/internal/process"

	"charm.land/lipgloss/v2"
)

func TestProcessListViewportScrolling(t *testing.T) {
	pl := NewProcessList()
	pl.Height = 5 // 1 line for header, 4 lines for items
	pl.Width = 40
	pl.Focused = true

	// Add 10 dummy profiles
	for i := 1; i <= 10; i++ {
		pl.Profiles = append(pl.Profiles, process.NewProfile(
			fmt.Sprintf("Proc-%d", i),
			process.MatchProcessName,
			fmt.Sprintf("proc%d", i),
			true,
		))
	}

	availableHeight := pl.Height - 1 // 4

	// Cursor at 0: Offset should be 0
	pl.Cursor = 0
	pl.Render()
	if pl.Offset != 0 {
		t.Errorf("expected offset 0, got %d", pl.Offset)
	}

	// Move cursor to index 5 (6th item, beyond available 4 lines)
	pl.Cursor = 5
	out := pl.Render()
	expectedOffset := 5 - availableHeight + 1 // 2
	if pl.Offset != expectedOffset {
		t.Errorf("expected offset %d, got %d", expectedOffset, pl.Offset)
	}

	// Verify rendered output height strictly does not exceed pl.Height
	h := lipgloss.Height(out)
	if h > pl.Height {
		t.Errorf("rendered height %d exceeds component height %d", h, pl.Height)
	}

	// Move cursor to last item (9)
	pl.Cursor = 9
	out = pl.Render()
	expectedLastOffset := 9 - availableHeight + 1 // 6
	if pl.Offset != expectedLastOffset {
		t.Errorf("expected offset %d, got %d", expectedLastOffset, pl.Offset)
	}
	h = lipgloss.Height(out)
	if h > pl.Height {
		t.Errorf("rendered height %d exceeds component height %d", h, pl.Height)
	}
}
