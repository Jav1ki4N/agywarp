package clash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectWithMockVergeDir(t *testing.T) {
	tmpDir := t.TempDir()
	profilesSubdir := filepath.Join(tmpDir, "profiles")
	if err := os.MkdirAll(profilesSubdir, 0755); err != nil {
		t.Fatalf("creating profiles subdir: %v", err)
	}

	// 1. Mock profiles.yaml
	mockProfilesYAML := `current: testProfileUID
items:
- uid: testProfileUID
  type: remote
  name: MockAirport
  file: testProfileUID.yaml
  option:
    rules: mockRules
    proxies: mockProxies
`
	if err := os.WriteFile(filepath.Join(tmpDir, "profiles.yaml"), []byte(mockProfilesYAML), 0644); err != nil {
		t.Fatalf("writing profiles.yaml: %v", err)
	}

	// 2. Mock mockProxies.yaml
	mockProxiesYAML := `prepend:
  - name: "WARP-LOCAL"
    type: socks5
    server: 127.0.0.1
    port: 40000
    udp: true
`
	if err := os.WriteFile(filepath.Join(profilesSubdir, "mockProxies.yaml"), []byte(mockProxiesYAML), 0644); err != nil {
		t.Fatalf("writing mockProxies.yaml: %v", err)
	}

	// 3. Mock mockRules.yaml
	mockRulesYAML := `prepend:
  - 'PROCESS-NAME,warp-svc,🚀节点选择'
  - 'PROCESS-NAME,agy,WARP-LOCAL'
`
	if err := os.WriteFile(filepath.Join(profilesSubdir, "mockRules.yaml"), []byte(mockRulesYAML), 0644); err != nil {
		t.Fatalf("writing mockRules.yaml: %v", err)
	}

	mgr := &SystemdManager{
		BaseDir:    tmpDir,
		SocketPath: "/tmp/non-existent-mock.sock",
	}

	insp, err := mgr.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}

	if !insp.Installed {
		t.Errorf("expected Installed=true")
	}
	if insp.ActiveProfileUID != "testProfileUID" {
		t.Errorf("expected ActiveProfileUID=testProfileUID, got %s", insp.ActiveProfileUID)
	}
	if insp.ActiveProfileName != "MockAirport" {
		t.Errorf("expected ActiveProfileName=MockAirport, got %s", insp.ActiveProfileName)
	}
	if !insp.HasWarpLocalProxy {
		t.Errorf("expected HasWarpLocalProxy=true")
	}
	if !insp.HasWarpSvcGuardRule {
		t.Errorf("expected HasWarpSvcGuardRule=true")
	}
	if !strings.HasSuffix(insp.ProxyExtensionFile, "mockProxies.yaml") {
		t.Errorf("expected ProxyExtensionFile ending in mockProxies.yaml, got %s", insp.ProxyExtensionFile)
	}
	if !strings.HasSuffix(insp.RuleExtensionFile, "mockRules.yaml") {
		t.Errorf("expected RuleExtensionFile ending in mockRules.yaml, got %s", insp.RuleExtensionFile)
	}
}

func TestInspectRealSystem(t *testing.T) {
	mgr := NewManager()
	insp, err := mgr.Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}

	if insp.Installed {
		t.Logf("Detected Verge BaseDir: %s", insp.VergeDir)
		t.Logf("Active Profile: %s (%s)", insp.ActiveProfileName, insp.ActiveProfileUID)
		t.Logf("Proxy Ext: %s (HasWarpLocal: %v)", insp.ProxyExtensionFile, insp.HasWarpLocalProxy)
		t.Logf("Rule Ext: %s (HasGuardRule: %v)", insp.RuleExtensionFile, insp.HasWarpSvcGuardRule)
		t.Logf("Socket available: %v (Version: %s, LiveRules: %d, WarpRules: %d)",
			insp.SocketAvailable, insp.MihomoVersion, insp.LiveRulesCount, insp.WarpRulesCount)
	}
}

func TestInjectRulesIntoClashVerge(t *testing.T) {
	orig := `port: 7890
rules:
- PROCESS-NAME,warp-svc,🚀节点选择
- PROCESS-NAME,agy,WARP-LOCAL
- DOMAIN-SUFFIX,google.com,DIRECT
`
	rules := []string{
		"PROCESS-NAME,chrome,WARP-LOCAL",
		"PROCESS-PATH,/usr/bin/google-chrome,WARP-LOCAL",
		"PROCESS-NAME,agy,WARP-LOCAL", // duplicate, should be skipped
	}

	injected, count := InjectRulesIntoClashVerge(orig, rules)
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}

	if !strings.Contains(injected, InjectStartMarker) || !strings.Contains(injected, InjectEndMarker) {
		t.Fatalf("expected injected markers in output:\n%s", injected)
	}

	lines := strings.Split(injected, "\n")
	warpSvcIdx := -1
	markerIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "warp-svc") {
			warpSvcIdx = i
		}
		if strings.TrimSpace(l) == InjectStartMarker {
			markerIdx = i
		}
	}

	if markerIdx != warpSvcIdx+1 {
		t.Fatalf("expected InjectStartMarker immediately after warp-svc rule (at %d), got at %d", warpSvcIdx+1, markerIdx)
	}

	// Test stripping restores exactly
	stripped := StripInjectedRules(injected)
	if stripped != orig {
		t.Fatalf("expected stripped content to match original exactly, got:\n%s", stripped)
	}
}

func TestInjectRulesIntoRuleExtension(t *testing.T) {
	orig := `# Enhancement
prepend:
  - 'PROCESS-NAME,warp-svc,🚀节点选择'
  - 'PROCESS-NAME,agy,WARP-LOCAL'
append: []
`
	rules := []string{
		"PROCESS-NAME,chrome,WARP-LOCAL",
	}

	injected, count := InjectRulesIntoRuleExtension(orig, rules)
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}

	if !strings.Contains(injected, "  - 'PROCESS-NAME,chrome,WARP-LOCAL'") {
		t.Fatalf("expected formatted rule in extension:\n%s", injected)
	}

	stripped := StripInjectedRules(injected)
	if stripped != orig {
		t.Fatalf("expected stripped content to match original:\n%s", stripped)
	}
}

func TestInjectAndRestoreDisk(t *testing.T) {
	tmpDir := t.TempDir()
	profilesDir := filepath.Join(tmpDir, "profiles")
	_ = os.MkdirAll(profilesDir, 0755)

	profilesYAML := `current: testProf
items:
- uid: testProf
  type: remote
  name: Test
  option:
    rules: testRules
`
	_ = os.WriteFile(filepath.Join(tmpDir, "profiles.yaml"), []byte(profilesYAML), 0644)

	clashVergeYAML := `rules:
- PROCESS-NAME,warp-svc,DIRECT
- PROCESS-NAME,agy,WARP-LOCAL
`
	clashVergePath := filepath.Join(tmpDir, "clash-verge.yaml")
	_ = os.WriteFile(clashVergePath, []byte(clashVergeYAML), 0644)

	rulesExtPath := filepath.Join(profilesDir, "testRules.yaml")
	rulesExtYAML := `prepend:
  - 'PROCESS-NAME,warp-svc,DIRECT'
`
	_ = os.WriteFile(rulesExtPath, []byte(rulesExtYAML), 0644)

	mgr := &SystemdManager{
		BaseDir:    tmpDir,
		SocketPath: "/tmp/non-existent-sock-for-test.sock",
	}

	// Direct injection without socket reload
	data, _ := os.ReadFile(clashVergePath)
	mod, count := InjectRulesIntoClashVerge(string(data), []string{"PROCESS-NAME,chrome,WARP-LOCAL"})
	if count != 1 {
		t.Fatalf("expected count=1, got %d", count)
	}
	_ = os.WriteFile(clashVergePath+".agywarp.bak", data, 0644)
	_ = os.WriteFile(clashVergePath, []byte(mod), 0644)

	// Verify modified
	afterMod, _ := os.ReadFile(clashVergePath)
	if !strings.Contains(string(afterMod), "chrome") {
		t.Fatalf("expected chrome in modified clash-verge.yaml")
	}

	// Restore files manually as RestoreRules does
	bakData, err := os.ReadFile(clashVergePath + ".agywarp.bak")
	if err != nil {
		t.Fatalf("backup not found: %v", err)
	}
	_ = os.WriteFile(clashVergePath, bakData, 0644)
	_ = os.Remove(clashVergePath + ".agywarp.bak")

	restored, _ := os.ReadFile(clashVergePath)
	if string(restored) != clashVergeYAML {
		t.Fatalf("expected restored to match original clash-verge.yaml")
	}
	if _, err := os.Stat(clashVergePath + ".agywarp.bak"); !os.IsNotExist(err) {
		t.Fatalf("expected backup to be removed")
	}
	_ = mgr
}
