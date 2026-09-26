package components

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestConsoleWrapPreservesDiagnosticAndScrolls(t *testing.T) {
	c := NewConsole()
	c.Clear()
	c.SetSize(48, 12)
	c.AddLog("WARN", "API metadata: CREDENTIALS_MISSING service=cloudcode-pa.googleapis.com method=PredictionService.FetchAvailableModels")
	rendered := c.Render()
	if strings.Contains(rendered, "…") || !strings.Contains(rendered, "API metadata:") {
		t.Fatalf("diagnostic was truncated: %s", rendered)
	}
	if lipgloss.Height(rendered) != 12 || c.totalLines < 2 {
		t.Fatalf("invalid wrapped layout: %s", rendered)
	}
	for _, line := range strings.Split(rendered, "\n") {
		if lipgloss.Width(line) > 48 {
			t.Fatalf("line overflow: %s", line)
		}
	}
	c.Clear()
	c.SetSize(80, 5)
	for i := 0; i < 20; i++ {
		c.AddLog("INFO", fmt.Sprintf("record-%02d", i))
	}
	if !strings.Contains(c.Render(), "record-19") {
		t.Fatal("latest log missing")
	}
	c.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if strings.Contains(c.Render(), "record-19") {
		t.Fatal("page up did not move away from tail")
	}
	before := c.Render()
	c.AddLog("INFO", "record-20")
	if c.Render() != before {
		t.Fatal("new log moved the historical view")
	}
	c.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !strings.Contains(c.Render(), "record-20") {
		t.Fatal("End did not restore tail")
	}
}

func TestConsoleHeightStrictlyMaintained(t *testing.T) {
	c := NewConsole()
	c.Width = 60
	c.Height = 8

	// Add 50 long logs that would normally wrap and cause height overflow
	for i := 1; i <= 50; i++ {
		c.AddLog("ERR", fmt.Sprintf("Log %d: Path does not exist in system: /home/user/very/deeply/nested/path/to/executable/binary/that/definitely/exceeds/the/console/width (%d)", i, i*100))
		c.AddLog("WARN", fmt.Sprintf("Warning %d: A long multiline-like\nmessage that should stay\r\non a single line", i))
		c.AddLog("OK", fmt.Sprintf("Success %d: Validated process path", i))
	}

	rendered := c.Render()
	actualHeight := lipgloss.Height(rendered)
	if actualHeight != 8 {
		t.Fatalf("expected Console.Render height to be strictly 8, got %d", actualHeight)
	}

	lines := strings.Split(rendered, "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 lines in output, got %d", len(lines))
	}

	// Verify width is respected on all lines
	for idx, line := range lines {
		w := lipgloss.Width(line)
		if w > 60 {
			t.Errorf("line %d width %d exceeds console width 60: %q", idx+1, w, line)
		}
	}

	// Test height change to 12
	c.Height = 12
	rendered12 := c.Render()
	actualHeight12 := lipgloss.Height(rendered12)
	if actualHeight12 != 12 {
		t.Fatalf("expected Console.Render height to be strictly 12, got %d", actualHeight12)
	}
}

func TestConsoleSpinner(t *testing.T) {
	c := NewConsole()
	c.Width = 60
	c.Height = 6

	// 1. When Active and SpinnerView is set (using slash format)
	c.Active = true
	c.SpinnerView = "/"
	c.AddLog("INFO", "Refreshing network & tunnel status...")

	rendered := c.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}

	// Verify header does NOT contain spinner
	if strings.Contains(lines[0], "/") {
		t.Errorf("expected header NOT to contain spinner frame, got: %q", lines[0])
	}

	// Verify in-progress log line contains spinner
	foundLogWithSpinner := false
	for _, l := range lines {
		if strings.Contains(l, "Refreshing network & tunnel status...") && strings.Contains(l, "/") {
			foundLogWithSpinner = true
			break
		}
	}
	if !foundLogWithSpinner {
		t.Errorf("expected active log entry to contain spinner '/', got:\n%s", rendered)
	}

	// 2. When Active is false and SpinnerView is cleared
	c.Active = false
	c.SpinnerView = ""
	c.AddLog("OK", "WARP exit verified: 104.28.195.192 (US, Colo: LAX, 35ms)")

	renderedDone := c.Render()
	linesDone := strings.Split(renderedDone, "\n")
	if strings.Contains(linesDone[0], "/") {
		t.Errorf("expected header NOT to contain spinner when done, got: %q", linesDone[0])
	}
}
