package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLoadDefaultsAndSave(t *testing.T) {
	// Create temporary directory for testing store
	tmpDir, err := os.MkdirTemp("", "agywarp-store-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "profiles.json")
	store, err := NewStore(filePath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// 1. First Load on non-existent file should initialize with default profiles and persist
	profiles, err := store.Load()
	if err != nil {
		t.Fatalf("unexpected error on first Load: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("expected default profiles on first load, got 0")
	}

	// Verify file was actually created
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected profiles.json to be created on disk, err: %v", err)
	}

	// 2. Modify and Save
	newProfile := NewProfile("TestApp", MatchProcessName, "testapp", true)
	profiles = append(profiles, newProfile)
	if err := store.Save(profiles); err != nil {
		t.Fatalf("failed to save profiles: %v", err)
	}

	// 3. Reload and verify persistence
	reloadedStore, err := NewStore(filePath)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Load()
	if err != nil {
		t.Fatalf("failed to reload profiles: %v", err)
	}
	if len(reloaded) != len(profiles) {
		t.Fatalf("expected %d profiles, got %d", len(profiles), len(reloaded))
	}

	found := false
	for _, p := range reloaded {
		if p.Label == "TestApp" && p.Enabled {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected newly added profile 'TestApp' to be persisted and loaded")
	}
}

func TestProfileToMihomoRules(t *testing.T) {
	p := NewGroupProfile("Antigravity", []Matcher{
		{Kind: MatchProcessName, Pattern: "agy"},
		{Kind: MatchExecutablePath, Pattern: "/home/i4N/.gemini/bin/agy"},
		{Kind: MatchDomain, Pattern: "cloudcode-pa.googleapis.com"},
		{Kind: MatchDomainSuffix, Pattern: "antigravity.google"},
	}, true)

	rules := p.ToMihomoRules("WARP-LOCAL")
	if len(rules) != 4 {
		t.Fatalf("expected 4 rules, got %d", len(rules))
	}
	if rules[0] != "PROCESS-NAME,agy,WARP-LOCAL" {
		t.Errorf("unexpected rule 0: %s", rules[0])
	}
	if rules[1] != "PROCESS-PATH,/home/i4N/.gemini/bin/agy,WARP-LOCAL" {
		t.Errorf("unexpected rule 1: %s", rules[1])
	}
	if rules[2] != "DOMAIN,cloudcode-pa.googleapis.com,WARP-LOCAL" {
		t.Errorf("unexpected rule 2: %s", rules[2])
	}
	if rules[3] != "DOMAIN-SUFFIX,antigravity.google,WARP-LOCAL" {
		t.Errorf("unexpected rule 3: %s", rules[3])
	}

	// Disabled profile returns nil
	p.Enabled = false
	if disabledRules := p.ToMihomoRules("WARP-LOCAL"); disabledRules != nil {
		t.Fatalf("expected nil rules for disabled profile, got %v", disabledRules)
	}
}
