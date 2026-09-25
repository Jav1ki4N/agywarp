package process

import (
	"context"
	"os"
	"testing"
)

func TestLinuxScanner(t *testing.T) {
	scanner := NewScanner()
	procs, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(procs) == 0 {
		t.Logf("no processes found for user %d (may happen in some container envs)", os.Getuid())
		return
	}

	t.Logf("discovered %d processes for current user", len(procs))

	// Verify our own process is NOT included (scanner skips myPID)
	myPID := os.Getpid()
	for _, p := range procs {
		if p.PID == myPID {
			t.Errorf("expected current process PID %d to be excluded, but found in scan results", myPID)
		}
	}
}

func TestProfileMatching(t *testing.T) {
	profiles := DefaultProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected default profiles, got empty")
	}

	fakeRunning := []RunningProcess{
		{PID: 100, Name: "agy", Executable: "/home/test/agy"},
		{PID: 101, Name: "agy", Executable: "/home/test/agy"},
		{PID: 200, Name: "bash", Executable: "/usr/bin/bash"},
	}

	count := CountRunningInstances(profiles[0], fakeRunning)
	if count != 2 {
		t.Errorf("expected 2 instances of %s, got %d", profiles[0].Label, count)
	}
}

func TestProcessNameDoesNotCountHelperProcesses(t *testing.T) {
	profile := NewProfile("Google Chrome", MatchProcessName, "chrome", false)
	running := []RunningProcess{
		{PID: 1, Name: "chrome", Executable: "/opt/google/chrome"},
		{PID: 2, Name: "chrome_crashpad", Executable: "/opt/QQ/chrome_crashpad_handler"},
	}
	if got := CountRunningInstances(profile, running); got != 1 {
		t.Fatalf("expected only exact chrome process, got %d", got)
	}
}
