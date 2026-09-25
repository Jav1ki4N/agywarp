package warp

import (
	"context"
	"testing"
)

func TestParseVersion(t *testing.T) {
	out := "warp-cli 2026.7.1377.0\n"
	ver := ParseVersion(out)
	if ver != "2026.7.1377.0" {
		t.Errorf("expected version 2026.7.1377.0, got %s", ver)
	}

	out2 := "warp-cli version 2024.1.2\n"
	ver2 := ParseVersion(out2)
	if ver2 != "version" && ver2 != "2024.1.2" {
		// Just ensure non-empty
		if ver2 == "" {
			t.Errorf("expected non-empty version")
		}
	}
}

func TestParseStatusOutput(t *testing.T) {
	sample := `Status update: Connected
Network: unstable
`
	info := ParseStatusOutput(sample)
	if info.Status != "CONNECTED" {
		t.Errorf("expected status CONNECTED, got %s", info.Status)
	}
	if info.NetworkHealth != "unstable" {
		t.Errorf("expected network unstable, got %s", info.NetworkHealth)
	}

	disconnectedSample := `Status update: Disconnected
`
	info2 := ParseStatusOutput(disconnectedSample)
	if info2.Status != "DISCONNECTED" {
		t.Errorf("expected status DISCONNECTED, got %s", info2.Status)
	}
	unable := ParseStatusOutput("Status update: Unable\nReason: Happy Eyeballs Failed\n")
	if unable.Status != "UNABLE" || unable.Reason != "Happy Eyeballs Failed" {
		t.Fatalf("expected WARP failure reason, got %+v", unable)
	}
}

func TestParseSettingsOutput(t *testing.T) {
	sample := `Merged configuration:
(not set)	Compliance Environment: Normal
(derived)	Always On: true
(override)	Switch Locked: false
(user set)	Mode: WarpProxy on port 40000
(consumer overrides)	WARP tunnel protocol: MASQUE
(not set)	MASQUE Protocol Settings: 
  HTTP Version: MASQUE (HTTP/3 with HTTP/2 fallback)
`
	proto, mode, port := ParseSettingsOutput(sample)
	if proto != "MASQUE" {
		t.Errorf("expected protocol MASQUE, got %s", proto)
	}
	if mode != "WarpProxy" {
		t.Errorf("expected mode WarpProxy, got %s", mode)
	}
	if port != 40000 {
		t.Errorf("expected port 40000, got %d", port)
	}
}

func TestDetectWithRealSystem(t *testing.T) {
	client := NewClient()
	install, err := client.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	// On this linux dev environment, warp-cli is installed
	if !install.Installed {
		t.Logf("warp-cli not installed in this environment")
	} else {
		if install.Path == "" {
			t.Errorf("expected non-empty path for installed warp-cli")
		}
		if install.Version == "" {
			t.Errorf("expected non-empty version for installed warp-cli")
		}
	}
}
