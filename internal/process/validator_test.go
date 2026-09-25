package process

import (
	"os"
	"testing"
)

func TestValidatePathExisting(t *testing.T) {
	// 1. Existing command in PATH (e.g., sh)
	res, err := ValidatePath("sh", nil)
	if err != nil || !res.Exists {
		t.Fatalf("expected 'sh' to exist, got res=%v, err=%v", res, err)
	}

	// 2. Existing path on disk (e.g., /bin or current working dir)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	res, err = ValidatePath(wd, nil)
	if err != nil || !res.Exists {
		t.Fatalf("expected current working dir to exist, got res=%v, err=%v", res, err)
	}

	// 3. Tilde path expansion
	home, err := os.UserHomeDir()
	if err == nil {
		res, err = ValidatePath("~", nil)
		if err != nil || !res.Exists || res.ResolvedPath != home {
			t.Fatalf("expected '~' to expand to home dir %s, got res=%v, err=%v", home, res, err)
		}
	}
}

func TestValidatePathNonExistent(t *testing.T) {
	// 1. Non-existent path
	res, err := ValidatePath("/non/existent/path/for/agywarp/testing/12345", nil)
	if err == nil || res.Exists {
		t.Fatalf("expected non-existent path to fail, got res=%v, err=%v", res, err)
	}

	// 2. Non-existent bare command
	res, err = ValidatePath("completely_fake_command_xyz_98765", nil)
	if err == nil || res.Exists {
		t.Fatalf("expected non-existent command to fail, got res=%v, err=%v", res, err)
	}

	// 3. Empty string
	res, err = ValidatePath("   ", nil)
	if err == nil || res.Exists {
		t.Fatalf("expected empty string to fail, got res=%v, err=%v", res, err)
	}
}

func TestValidatePathRunningProcess(t *testing.T) {
	// Simulated running process
	running := []RunningProcess{
		{
			PID:        9999,
			Name:       "my-custom-agent",
			Executable: "/opt/custom/bin/my-custom-agent",
		},
	}

	// Matching by name
	res, err := ValidatePath("my-custom-agent", running)
	if err != nil || !res.Exists {
		t.Fatalf("expected running process to match by name, got res=%v, err=%v", res, err)
	}

	// Matching by executable path
	res, err = ValidatePath("/opt/custom/bin/my-custom-agent", running)
	if err != nil || !res.Exists {
		t.Fatalf("expected running process to match by exe, got res=%v, err=%v", res, err)
	}
}

func TestValidateDomain(t *testing.T) {
	// Full domain
	res, err := ValidatePath("cloudcode-pa.googleapis.com", nil)
	if err != nil || !res.Exists || res.Kind != MatchDomain {
		t.Fatalf("expected cloudcode-pa.googleapis.com to match as domain, got res=%v, err=%v", res, err)
	}

	// Domain suffix wildcard
	res, err = ValidatePath("*.antigravity.google", nil)
	if err != nil || !res.Exists || res.Kind != MatchDomainSuffix || res.ResolvedPath != "antigravity.google" {
		t.Fatalf("expected *.antigravity.google to match as domain_suffix, got res=%v, err=%v", res, err)
	}

	// Domain suffix leading dot
	res, err = ValidatePath(".google.com", nil)
	if err != nil || !res.Exists || res.Kind != MatchDomainSuffix || res.ResolvedPath != "google.com" {
		t.Fatalf("expected .google.com to match as domain_suffix, got res=%v, err=%v", res, err)
	}
}
