package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type ValidationResult struct {
	Exists       bool
	ResolvedPath string
	Kind         MatchKind
	Details      string
}

// ExpandHome expands leading ~/ or ~ to user's home directory.
func ExpandHome(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	} else if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// ValidatePath checks if rawPath really exists in the system as a file,
// command in PATH, common binary location, or active running process.
func ValidatePath(rawPath string, running []RunningProcess) (ValidationResult, error) {
	trimmed := strings.TrimSpace(rawPath)
	trimmed = strings.Trim(trimmed, "\"'")
	if trimmed == "" {
		return ValidationResult{Exists: false}, fmt.Errorf("path cannot be empty")
	}

	// Case 0: Domain suffix or wildcard domain rule
	if strings.HasPrefix(trimmed, "*.") || (strings.HasPrefix(trimmed, ".") && !strings.Contains(trimmed, "/")) {
		suffix := strings.TrimPrefix(strings.TrimPrefix(trimmed, "*"), ".")
		return ValidationResult{
			Exists:       true,
			ResolvedPath: suffix,
			Kind:         MatchDomainSuffix,
			Details:      "domain suffix rule",
		}, nil
	}

	// Case 0b: Full domain name (e.g. cloudcode-pa.googleapis.com)
	if strings.Contains(trimmed, ".") && !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, `\`) {
		if _, err := exec.LookPath(trimmed); err != nil {
			parts := strings.Split(trimmed, ".")
			if len(parts) >= 2 && len(parts[len(parts)-1]) >= 2 {
				return ValidationResult{
					Exists:       true,
					ResolvedPath: trimmed,
					Kind:         MatchDomain,
					Details:      "domain routing rule",
				}, nil
			}
		}
	}

	expanded := ExpandHome(trimmed)

	// Case 1: Path contains directory separators or relative prefix
	if strings.Contains(expanded, "/") || strings.Contains(expanded, `\`) || strings.HasPrefix(expanded, ".") {
		cleaned := filepath.Clean(expanded)
		if info, err := os.Stat(cleaned); err == nil {
			desc := "executable binary exists"
			if info.IsDir() {
				desc = "directory exists"
			}
			return ValidationResult{
				Exists:       true,
				ResolvedPath: cleaned,
				Kind:         MatchExecutablePath,
				Details:      desc,
			}, nil
		}

		// Check if it's a glob pattern that matches any files
		if strings.ContainsAny(cleaned, "*?[") {
			if matches, _ := filepath.Glob(cleaned); len(matches) > 0 {
				return ValidationResult{
					Exists:       true,
					ResolvedPath: cleaned,
					Kind:         MatchExecutablePath,
					Details:      fmt.Sprintf("glob matches %d file(s)", len(matches)),
				}, nil
			}
		}

		// Check against running processes by executable path
		for _, p := range running {
			if p.Executable == cleaned {
				return ValidationResult{
					Exists:       true,
					ResolvedPath: cleaned,
					Kind:         MatchExecutablePath,
					Details:      fmt.Sprintf("active process (PID %d)", p.PID),
				}, nil
			}
		}

		return ValidationResult{
			Exists:       false,
			ResolvedPath: cleaned,
			Kind:         MatchExecutablePath,
		}, fmt.Errorf("file not found on disk: %s", cleaned)
	}

	// Case 2: Bare command or process name (e.g. "agy", "curl", "chrome")
	// 2a. Look in PATH
	if p, err := exec.LookPath(trimmed); err == nil {
		return ValidationResult{
			Exists:       true,
			ResolvedPath: p,
			Kind:         MatchProcessName,
			Details:      fmt.Sprintf("found in PATH: %s", p),
		}, nil
	}

	// 2b. Check common user & system bin directories
	if home, err := os.UserHomeDir(); err == nil {
		commonDirs := []string{
			filepath.Join(home, ".local", "bin"),
			filepath.Join(home, ".gemini", "bin"),
			filepath.Join(home, ".cargo", "bin"),
			"/usr/local/bin",
			"/usr/bin",
			"/bin",
			"/usr/sbin",
			"/sbin",
		}
		for _, dir := range commonDirs {
			candidate := filepath.Join(dir, trimmed)
			if _, err := os.Stat(candidate); err == nil {
				return ValidationResult{
					Exists:       true,
					ResolvedPath: candidate,
					Kind:         MatchProcessName,
					Details:      fmt.Sprintf("found binary: %s", candidate),
				}, nil
			}
		}
	}

	// 2c. Check currently running processes (or live scan if running list is empty)
	if len(running) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		scanner := NewScanner()
		if procs, err := scanner.Scan(ctx); err == nil {
			running = procs
		}
	}

	for _, p := range running {
		if p.Name == trimmed || filepath.Base(p.Executable) == trimmed {
			return ValidationResult{
				Exists:       true,
				ResolvedPath: p.Executable,
				Kind:         MatchProcessName,
				Details:      fmt.Sprintf("running process: %s (PID %d)", p.Executable, p.PID),
			}, nil
		}
	}

	return ValidationResult{
		Exists:       false,
		ResolvedPath: trimmed,
		Kind:         MatchProcessName,
	}, fmt.Errorf("no executable or running process matches '%s'", trimmed)
}
