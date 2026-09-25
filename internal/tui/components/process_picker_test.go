package components

import (
	"testing"

	"agywarp/internal/process"
)

func TestProcessPickerGrouping(t *testing.T) {
	picker := NewProcessPicker()
	picker.Height = 10
	picker.Width = 60
	picker.Open = true

	// Simulate discovery of two agy instances with different paths
	fakeProcs := []process.RunningProcess{
		{PID: 1001, Name: "agy", Executable: "/home/i4N/.gemini/bin/agy", Command: []string{"agy", "--hub"}},
		{PID: 1002, Name: "agy", Executable: "/home/i4N/.local/bin/agy", Command: []string{"agy"}},
		{PID: 2001, Name: "chrome", Executable: "/opt/google/chrome/chrome", Command: []string{"chrome"}},
		{PID: 2002, Name: "chrome", Executable: "/opt/google/chrome/chrome", Command: []string{"chrome", "--type=renderer"}},
	}

	picker.SetRunningProcesses(fakeProcs)

	if len(picker.AllGroups) != 2 {
		t.Fatalf("expected 2 distinct groups (agy and chrome), got %d", len(picker.AllGroups))
	}

	// Verify agy group has 2 distinct executables
	var agyGroup *ProcessGroupCandidate
	for _, g := range picker.AllGroups {
		if g.Name == "agy" {
			agyGroup = g
			break
		}
	}

	if agyGroup == nil {
		t.Fatal("expected agy group to exist")
	}

	if len(agyGroup.Executables) != 2 {
		t.Fatalf("expected 2 unique executables for agy group, got %d", len(agyGroup.Executables))
	}

	if agyGroup.TotalInst != 2 {
		t.Errorf("expected 2 total instances, got %d", agyGroup.TotalInst)
	}

	// Both should be selected by default
	if !agyGroup.Executables[0].Selected || !agyGroup.Executables[1].Selected {
		t.Error("expected all executables in group to be selected by default")
	}

	// Verify chrome group deduplicated into 1 executable with 2 instances
	var chromeGroup *ProcessGroupCandidate
	for _, g := range picker.AllGroups {
		if g.Name == "chrome" {
			chromeGroup = g
			break
		}
	}
	if chromeGroup == nil {
		t.Fatal("expected chrome group to exist")
	}
	if len(chromeGroup.Executables) != 1 {
		t.Fatalf("expected chrome to have 1 unique executable, got %d", len(chromeGroup.Executables))
	}
	if chromeGroup.TotalInst != 2 {
		t.Errorf("expected chrome to have 2 total instances, got %d", chromeGroup.TotalInst)
	}
}
