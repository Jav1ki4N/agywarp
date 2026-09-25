package warp

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Installation represents warp-cli binary detection results.
type Installation struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
}

// StatusInfo encapsulates parsed runtime WARP status and configuration.
type StatusInfo struct {
	Status        string `json:"status"` // CONNECTED, DISCONNECTED, CONNECTING, etc.
	Reason        string `json:"reason"`
	NetworkHealth string `json:"network"`  // stable, unstable, etc.
	Protocol      string `json:"protocol"` // MASQUE, WireGuard, etc.
	Mode          string `json:"mode"`     // WarpProxy, Tunnel, etc.
	ProxyPort     int    `json:"proxy_port"`
}

// Client abstracts operations interacting with the Cloudflare WARP daemon.
type Client interface {
	Detect(ctx context.Context) (Installation, error)
	Status(ctx context.Context) (StatusInfo, error)
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
	EnsureSettings(ctx context.Context) error
}

// CLIClient interacts with warp-cli via command execution.
type CLIClient struct {
	binaryPath string
}

// NewClient constructs a new WARP CLI client.
func NewClient(customPath ...string) Client {
	bin := "warp-cli"
	if len(customPath) > 0 && customPath[0] != "" {
		bin = customPath[0]
	}
	return &CLIClient{binaryPath: bin}
}

// Detect checks if warp-cli exists in PATH and queries its version.
func (c *CLIClient) Detect(ctx context.Context) (Installation, error) {
	path, err := exec.LookPath(c.binaryPath)
	if err != nil {
		return Installation{Installed: false}, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, path, "--version")
	out, err := cmd.Output()
	if err != nil {
		// Found binary, but error getting version
		return Installation{
			Installed: true,
			Path:      path,
			Version:   "unknown",
		}, nil
	}

	ver := ParseVersion(string(out))
	return Installation{
		Installed: true,
		Path:      path,
		Version:   ver,
	}, nil
}

// Status queries and parses the active connection status and settings.
func (c *CLIClient) Status(ctx context.Context) (StatusInfo, error) {
	path, err := exec.LookPath(c.binaryPath)
	if err != nil {
		return StatusInfo{Status: "NOT INSTALLED"}, fmt.Errorf("warp-cli not found: %w", err)
	}

	execCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmdStatus := exec.CommandContext(execCtx, path, "status")
	outStatus, err := cmdStatus.Output()
	if err != nil {
		return StatusInfo{Status: "ERROR"}, fmt.Errorf("querying warp-cli status: %w", err)
	}

	info := ParseStatusOutput(string(outStatus))

	// Also query settings to retrieve protocol and proxy mode/port
	cmdSettings := exec.CommandContext(execCtx, path, "settings")
	outSettings, err := cmdSettings.Output()
	if err == nil {
		proto, mode, port := ParseSettingsOutput(string(outSettings))
		if proto != "" {
			info.Protocol = proto
		}
		if mode != "" {
			info.Mode = mode
		}
		if port > 0 {
			info.ProxyPort = port
		}
	}

	return info, nil
}

func (c *CLIClient) Connect(ctx context.Context) error {
	path, err := exec.LookPath(c.binaryPath)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "connect")
	return cmd.Run()
}

func (c *CLIClient) Disconnect(ctx context.Context) error {
	path, err := exec.LookPath(c.binaryPath)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, path, "disconnect")
	return cmd.Run()
}

func (c *CLIClient) EnsureSettings(ctx context.Context) error {
	// Mode and protocol checking
	return nil
}

// ParseVersion extracts version string from 'warp-cli --version' output.
func ParseVersion(output string) string {
	line := strings.TrimSpace(output)
	fields := strings.Fields(line)
	if len(fields) >= 2 && fields[0] == "warp-cli" {
		return fields[1]
	}
	return line
}

// ParseStatusOutput parses output of 'warp-cli status'.
func ParseStatusOutput(output string) StatusInfo {
	info := StatusInfo{
		Status:   "DISCONNECTED",
		Protocol: "---",
		Mode:     "---",
	}

	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if strings.HasPrefix(line, "Status update:") {
			st := strings.TrimSpace(strings.TrimPrefix(line, "Status update:"))
			info.Status = strings.ToUpper(st)
		} else if strings.HasPrefix(line, "Reason:") {
			info.Reason = strings.TrimSpace(strings.TrimPrefix(line, "Reason:"))
		} else if strings.HasPrefix(line, "Network:") {
			info.NetworkHealth = strings.TrimSpace(strings.TrimPrefix(line, "Network:"))
		}
	}
	return info
}

var proxyPortRe = regexp.MustCompile(`(?i)Mode:\s*WarpProxy\s+on\s+port\s+(\d+)`)
var protocolRe = regexp.MustCompile(`(?i)WARP tunnel protocol:\s*([A-Za-z0-9\-]+)`)

// ParseSettingsOutput extracts protocol, mode and port from 'warp-cli settings'.
func ParseSettingsOutput(output string) (protocol string, mode string, port int) {
	lines := strings.Split(output, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		if protocolRe.MatchString(line) {
			m := protocolRe.FindStringSubmatch(line)
			if len(m) > 1 {
				protocol = strings.TrimSpace(m[1])
			}
		}

		if proxyPortRe.MatchString(line) {
			mode = "WarpProxy"
			m := proxyPortRe.FindStringSubmatch(line)
			if len(m) > 1 {
				if p, err := strconv.Atoi(m[1]); err == nil {
					port = p
				}
			}
		} else if strings.Contains(line, "Mode:") && mode == "" {
			parts := strings.SplitN(line, "Mode:", 2)
			if len(parts) == 2 {
				mode = strings.TrimSpace(parts[1])
			}
		}
	}
	return protocol, mode, port
}

// RunLookPath is a helper wrapping exec.LookPath for easy testing or mock.
var RunLookPath = exec.LookPath

// IsWarpInstalled returns true if warp-cli is discovered in PATH.
func IsWarpInstalled() (bool, string) {
	path, err := RunLookPath("warp-cli")
	if err != nil {
		return false, ""
	}
	return true, path
}
