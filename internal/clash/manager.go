package clash

import (
	"agywarp/internal/proxymode"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manager abstracts interactions with Clash / Mihomo configuration and service lifecycle.
type Manager interface {
	Inspect(ctx context.Context) (Inspection, error)
	CheckConfig(ctx context.Context) (bool, error)
	EnsureRuntimeLoaded(ctx context.Context) error
	RestartService(ctx context.Context) error
	InjectRules(ctx context.Context, rules []string) (int, error)
	RestoreRules(ctx context.Context) error
	ReloadMihomo(ctx context.Context) error
	InjectProfileRules(ctx context.Context, proxyGroupName string) error
	Preflight(ctx context.Context) (RuntimeCheck, error)
	BootstrapRuntime(ctx context.Context, port int) error
	AbortBootstrap(ctx context.Context) error
	StartRuntime(ctx context.Context, rules []string, port int, connectedByUs bool) (int, error)
	StopRuntime(ctx context.Context) (bool, error)
	RecoverRuntime(ctx context.Context) error
	RuntimeRouteChanged(ctx context.Context) (string, error)
}

// SystemdManager manages Clash Verge / Mihomo via files and Unix socket API.
type SystemdManager struct {
	proxyMode  proxymode.Selection
	BaseDir    string
	SocketPath string
	HTTPClient *http.Client // optional controller transport for embedded clients and tests
}

// DefaultBaseDir returns the standard Clash Verge Rev configuration directory.
func DefaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "io.github.clash-verge-rev.clash-verge-rev")
}

// DefaultSocketPath returns the standard Mihomo external controller unix socket path.
func DefaultSocketPath() string {
	return "/tmp/verge/verge-mihomo.sock"
}

// NewManager constructs a new Clash manager instance.
func NewManager(customBaseDir ...string) Manager {
	base := DefaultBaseDir()
	if len(customBaseDir) > 0 && customBaseDir[0] != "" {
		base = customBaseDir[0]
	}
	return &SystemdManager{
		BaseDir:    base,
		SocketPath: DefaultSocketPath(),
	}
}

// Inspect performs a comprehensive, completely read-only diagnosis of Clash Verge Rev and Mihomo.
func (m *SystemdManager) Inspect(ctx context.Context) (Inspection, error) {
	report := Inspection{
		VergeDir:      m.BaseDir,
		SocketPath:    m.SocketPath,
		SummaryStatus: "NOT INSTALLED",
	}

	if m.BaseDir == "" {
		return report, fmt.Errorf("unable to determine clash-verge-rev base directory")
	}

	// 1. Check directory existence
	if _, err := os.Stat(m.BaseDir); os.IsNotExist(err) {
		return report, nil
	}
	report.Installed = true

	// 2. Parse profiles.yaml to identify the current active subscription
	profilesPath := filepath.Join(m.BaseDir, "profiles.yaml")
	if data, err := os.ReadFile(profilesPath); err == nil {
		var profs VergeProfiles
		if err := yaml.Unmarshal(data, &profs); err == nil {
			report.ActiveProfileUID = profs.Current
			for _, it := range profs.Items {
				if it.UID == profs.Current {
					report.ActiveProfileName = it.Name
					if it.Option != nil {
						if it.Option.Proxies != "" {
							report.ProxyExtensionFile = filepath.Join(m.BaseDir, "profiles", it.Option.Proxies+".yaml")
						}
						if it.Option.Rules != "" {
							report.RuleExtensionFile = filepath.Join(m.BaseDir, "profiles", it.Option.Rules+".yaml")
						}
					}
					break
				}
			}
		}
	}

	// 3. Inspect proxy extension for WARP-LOCAL proxy node
	if report.ProxyExtensionFile != "" {
		if data, err := os.ReadFile(report.ProxyExtensionFile); err == nil {
			var pExt ProxyExtension
			if err := yaml.Unmarshal(data, &pExt); err == nil {
				for _, p := range pExt.Prepend {
					if p.Name == "WARP-LOCAL" {
						report.HasWarpLocalProxy = true
						break
					}
				}
				if !report.HasWarpLocalProxy {
					for _, p := range pExt.Append {
						if p.Name == "WARP-LOCAL" {
							report.HasWarpLocalProxy = true
							break
						}
					}
				}
			}
		}
	}

	// Fallback check in generated clash-verge.yaml for WARP-LOCAL
	if !report.HasWarpLocalProxy {
		genPath := filepath.Join(m.BaseDir, "clash-verge.yaml")
		if data, err := os.ReadFile(genPath); err == nil {
			if strings.Contains(string(data), "name: WARP-LOCAL") || strings.Contains(string(data), `name: "WARP-LOCAL"`) {
				report.HasWarpLocalProxy = true
			}
		}
	}

	// 4. Inspect rule extension for warp-svc guard rule and WARP rules
	if report.RuleExtensionFile != "" {
		if data, err := os.ReadFile(report.RuleExtensionFile); err == nil {
			var rExt RuleExtension
			if err := yaml.Unmarshal(data, &rExt); err == nil {
				for _, r := range rExt.Prepend {
					if strings.Contains(r, "warp-svc") {
						report.HasWarpSvcGuardRule = true
						break
					}
				}
			}
		}
	}

	// Fallback check in generated clash-verge.yaml for warp-svc rule
	if !report.HasWarpSvcGuardRule {
		genPath := filepath.Join(m.BaseDir, "clash-verge.yaml")
		if data, err := os.ReadFile(genPath); err == nil {
			if strings.Contains(string(data), "PROCESS-NAME,warp-svc,") {
				report.HasWarpSvcGuardRule = true
			}
		}
	}

	// 5. Query Mihomo Unix socket API for live status
	m.querySocket(ctx, &report)

	// 6. Formulate summary status
	if !report.SocketAvailable {
		report.SummaryStatus = "CORE OFFLINE"
	} else if report.WarpRulesCount > 0 {
		report.SummaryStatus = fmt.Sprintf("SYNCED (%d rules)", report.WarpRulesCount)
	} else {
		report.SummaryStatus = "READY"
	}

	return report, nil
}

// querySocket queries Mihomo core's Unix domain socket for live version and rules.
func (m *SystemdManager) querySocket(ctx context.Context, report *Inspection) {
	if _, err := os.Stat(m.SocketPath); os.IsNotExist(err) {
		report.SocketAvailable = false
		return
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(dialCtx, "unix", m.SocketPath)
			},
		},
		Timeout: 2 * time.Second,
	}

	// 1. Query /version
	reqVer, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/version", nil)
	if err == nil {
		if resp, err := client.Do(reqVer); err == nil {
			defer resp.Body.Close()
			var vResp struct {
				Version string `json:"version"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&vResp); err == nil {
				report.MihomoVersion = vResp.Version
				report.SocketAvailable = true
			}
		}
	}

	if !report.SocketAvailable {
		return
	}

	// 2. Query /rules
	reqRules, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/rules", nil)
	if err == nil {
		if resp, err := client.Do(reqRules); err == nil {
			defer resp.Body.Close()
			var rResp struct {
				Rules []struct {
					Type    string `json:"type"`
					Payload string `json:"payload"`
					Proxy   string `json:"proxy"`
				} `json:"rules"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&rResp); err == nil {
				report.LiveRulesCount = len(rResp.Rules)
				warpRules := 0
				for _, r := range rResp.Rules {
					if r.Proxy == "WARP-LOCAL" || r.Proxy == runtimeProxy {
						warpRules++
					}
					if strings.EqualFold(r.Payload, "warp-svc") {
						report.HasWarpSvcGuardRule = true
					}
				}
				report.WarpRulesCount = warpRules
			}
		}
	}
}

func (m *SystemdManager) CheckConfig(ctx context.Context) (bool, error) {
	insp, err := m.Inspect(ctx)
	if err != nil {
		return false, err
	}
	return insp.HasWarpLocalProxy && insp.HasWarpSvcGuardRule, nil
}

func (m *SystemdManager) EnsureRuntimeLoaded(ctx context.Context) error {
	insp, err := m.Inspect(ctx)
	if err != nil {
		return err
	}
	if !insp.SocketAvailable {
		return fmt.Errorf("mihomo core socket is unavailable at %s", m.SocketPath)
	}
	return nil
}

func (m *SystemdManager) RestartService(ctx context.Context) error {
	// Preserved for interface compatibility; stage 3 will implement zero-downtime hot-reload API
	return nil
}

const (
	// InjectStartMarker marks the beginning of agywarp dynamic rules.
	InjectStartMarker = "# >>> [AGYWARP-DYNAMIC-RULES-START] >>>"
	// InjectEndMarker marks the end of agywarp dynamic rules.
	InjectEndMarker = "# <<< [AGYWARP-DYNAMIC-RULES-END] <<<"
)

// StripInjectedRules cleanly removes any previous dynamic injection marker block.
func StripInjectedRules(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == InjectStartMarker {
			inBlock = true
			continue
		}
		if trimmed == InjectEndMarker {
			inBlock = false
			continue
		}
		if !inBlock {
			result = append(result, line)
		}
	}
	return strings.Join(result, "\n")
}

// InjectRulesIntoClashVerge inserts rules into clash-verge.yaml after the warp-svc guard rule or under rules:.
func InjectRulesIntoClashVerge(content string, rules []string) (string, int) {
	cleanContent := StripInjectedRules(content)
	if len(rules) == 0 {
		return cleanContent, 0
	}

	lines := strings.Split(cleanContent, "\n")

	// Filter out rules already present outside injection
	var toInject []string
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		exists := false
		for _, line := range lines {
			if strings.Contains(line, r) {
				exists = true
				break
			}
		}
		if !exists {
			toInject = append(toInject, r)
		}
	}

	if len(toInject) == 0 {
		return cleanContent, 0
	}

	// Prepare injected block
	var block []string
	block = append(block, InjectStartMarker)
	for _, r := range toInject {
		block = append(block, "- "+r)
	}
	block = append(block, InjectEndMarker)

	// Find insertion index: look for warp-svc under rules:, or just after rules:
	insertIdx := -1
	rulesHeaderIdx := -1
	inRules := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "rules:" || strings.HasPrefix(trimmed, "rules:") {
			rulesHeaderIdx = i
			inRules = true
			continue
		}
		if inRules {
			if strings.Contains(line, "warp-svc") {
				insertIdx = i + 1
				break
			}
			// If we exit rules section (e.g. another top-level key)
			if len(line) > 0 && line[0] != ' ' && line[0] != '-' && line[0] != '#' && rulesHeaderIdx != -1 && i > rulesHeaderIdx {
				break
			}
		}
	}

	if insertIdx == -1 {
		if rulesHeaderIdx != -1 {
			insertIdx = rulesHeaderIdx + 1
		} else {
			insertIdx = len(lines)
		}
	}

	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, block...)
	newLines = append(newLines, lines[insertIdx:]...)

	return strings.Join(newLines, "\n"), len(toInject)
}

// InjectRulesIntoRuleExtension inserts rules into active rule extension yaml (e.g. r4uzTpqME3dW.yaml).
func InjectRulesIntoRuleExtension(content string, rules []string) (string, int) {
	cleanContent := StripInjectedRules(content)
	if len(rules) == 0 {
		return cleanContent, 0
	}

	lines := strings.Split(cleanContent, "\n")

	var toInject []string
	for _, r := range rules {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		exists := false
		for _, line := range lines {
			if strings.Contains(line, r) {
				exists = true
				break
			}
		}
		if !exists {
			toInject = append(toInject, r)
		}
	}

	if len(toInject) == 0 {
		return cleanContent, 0
	}

	var block []string
	block = append(block, InjectStartMarker)
	for _, r := range toInject {
		block = append(block, fmt.Sprintf("  - '%s'", r))
	}
	block = append(block, InjectEndMarker)

	insertIdx := -1
	prependHeaderIdx := -1
	inPrepend := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "prepend:" || strings.HasPrefix(trimmed, "prepend:") {
			prependHeaderIdx = i
			inPrepend = true
			continue
		}
		if inPrepend {
			if strings.Contains(line, "warp-svc") {
				insertIdx = i + 1
				break
			}
			if len(line) > 0 && line[0] != ' ' && line[0] != '-' && line[0] != '#' && prependHeaderIdx != -1 && i > prependHeaderIdx {
				break
			}
		}
	}

	if insertIdx == -1 {
		if prependHeaderIdx != -1 {
			insertIdx = prependHeaderIdx + 1
		} else {
			insertIdx = len(lines)
		}
	}

	var newLines []string
	newLines = append(newLines, lines[:insertIdx]...)
	newLines = append(newLines, block...)
	newLines = append(newLines, lines[insertIdx:]...)

	return strings.Join(newLines, "\n"), len(toInject)
}

// InjectRules dynamically injects process routing rules into Clash Verge and Mihomo.
func (m *SystemdManager) InjectRules(ctx context.Context, rules []string) (int, error) {
	return 0, fmt.Errorf("persistent rule injection is disabled; use StartRuntime")
}

func (m *SystemdManager) RestoreRules(ctx context.Context) error {
	return fmt.Errorf("persistent rule restoration is disabled; use StopRuntime")
}

// ReloadMihomo triggers external controller config reload via unix socket.
func (m *SystemdManager) ReloadMihomo(ctx context.Context) error {
	if _, err := os.Stat(m.SocketPath); os.IsNotExist(err) {
		return fmt.Errorf("mihomo controller socket not found at %s", m.SocketPath)
	}

	clashVergePath := filepath.Join(m.BaseDir, "clash-verge.yaml")
	payload, err := json.Marshal(map[string]string{
		"path": clashVergePath,
	})
	if err != nil {
		return fmt.Errorf("marshaling reload payload: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(dialCtx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(dialCtx, "unix", m.SocketPath)
			},
		},
		Timeout: 3 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", "http://localhost/configs?force=true", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating reload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("calling mihomo reload API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mihomo reload returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	return nil
}

func (m *SystemdManager) InjectProfileRules(ctx context.Context, proxyGroupName string) error {
	return nil
}

func (m *SystemdManager) SetProxyMode(mode proxymode.Mode) { m.proxyMode.Set(mode) }
