package pages

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"agywarp/internal/checker"
	"agywarp/internal/clash"
	"agywarp/internal/process"
	"agywarp/internal/tui/components"
	"agywarp/internal/tui/styles"
	"agywarp/internal/warp"

	tea "charm.land/bubbletea/v2"
)

type fixedWarpClient struct{ status warp.StatusInfo }

type testNodeManager struct {
	clash.Manager
	bootstrapCalls int
	abortCalls     int
}

func (m *testNodeManager) Preflight(context.Context) (clash.RuntimeCheck, error) {
	return clash.RuntimeCheck{}, nil
}

func (m *testNodeManager) BootstrapRuntime(context.Context, int) error {
	m.bootstrapCalls++
	return nil
}
func (m *testNodeManager) AbortBootstrap(context.Context) error {
	m.abortCalls++
	return nil
}

type testNodeWarpClient struct {
	fixedWarpClient
	connectCalls    int
	disconnectCalls int
}

func (c *testNodeWarpClient) Status(context.Context) (warp.StatusInfo, error) {
	return c.status, nil
}
func (c *testNodeWarpClient) Connect(context.Context) error {
	c.connectCalls++
	c.status.Status = "CONNECTED"
	return nil
}
func (c *testNodeWarpClient) Disconnect(context.Context) error {
	c.disconnectCalls++
	c.status.Status = "DISCONNECTED"
	return nil
}

type testNodeChecker struct{ verification *checker.ExitVerification }

func (c testNodeChecker) CheckPortListening(context.Context, string) bool { return true }
func (c testNodeChecker) VerifyExit(context.Context, string) (*checker.ExitVerification, error) {
	return c.verification, nil
}

func TestCurrentNodeRestoresState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		initial    string
		isWarp     bool
		wantErr    bool
		wantDialed int
	}{
		{"disconnected success", "DISCONNECTED", true, false, 1},
		{"disconnected invalid exit", "DISCONNECTED", false, true, 1},
		{"already connected", "CONNECTED", true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &testNodeManager{}
			client := &testNodeWarpClient{fixedWarpClient: fixedWarpClient{status: warp.StatusInfo{Status: tc.initial, Mode: "WarpProxy", ProxyPort: 40000}}}
			result, err := testCurrentNode(context.Background(), manager, client, testNodeChecker{&checker.ExitVerification{IsWarp: tc.isWarp}})
			if (err != nil) != tc.wantErr || (result != nil) == tc.wantErr {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if manager.bootstrapCalls != 1 || manager.abortCalls != 1 || client.connectCalls != tc.wantDialed || client.disconnectCalls != tc.wantDialed || client.status.Status != tc.initial {
				t.Fatalf("state not restored: manager=%+v client=%+v", manager, client)
			}
		})
	}
}

func (f fixedWarpClient) Detect(context.Context) (warp.Installation, error) {
	return warp.Installation{}, nil
}
func (f fixedWarpClient) Status(context.Context) (warp.StatusInfo, error) { return f.status, nil }
func (f fixedWarpClient) Connect(context.Context) error                   { return nil }
func (f fixedWarpClient) Disconnect(context.Context) error                { return nil }
func (f fixedWarpClient) EnsureSettings(context.Context) error            { return nil }

func TestWaitForWarpProxyReportsTerminalFailure(t *testing.T) {
	client := fixedWarpClient{status: warp.StatusInfo{Status: "UNABLE", Reason: "Happy Eyeballs Failed"}}
	err := waitForWarpProxy(context.Background(), client, checker.NewChecker(), 40000)
	if err == nil || !strings.Contains(err.Error(), "Happy Eyeballs Failed") || !strings.Contains(err.Error(), "UDP connectivity") {
		t.Fatalf("expected WARP failure reason, got %v", err)
	}
}

func TestWaitForWarpProxyReportsLastStateOnTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	client := fixedWarpClient{status: warp.StatusInfo{Status: "CONNECTING"}}
	err := waitForWarpProxy(ctx, client, checker.NewChecker(), 40000)
	if err == nil || !strings.Contains(err.Error(), "CONNECTING") {
		t.Fatalf("expected last WARP state, got %v", err)
	}
}

func TestHomeRenderFooterAlwaysAnchored(t *testing.T) {
	home := &Home{}
	home.Init()

	// Send WindowSizeMsg for standard 80x24 terminal
	home.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// 1. Add 100 heavy logs into console
	for i := 1; i <= 100; i++ {
		home.console.AddLog("ERR", fmt.Sprintf("Log %d: Path does not exist in system: /very/long/nonexistent/path/on/system/causing/overflow/issue/when/logging/a/lot (%d)", i, i*10))
	}

	rendered := home.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 24 {
		t.Fatalf("expected total Home.Render height to be strictly 24 lines, got %d", len(lines))
	}

	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, "STATUS") && !strings.Contains(lastLine, "tab:") && !strings.Contains(lastLine, "DOWN") {
		t.Fatalf("expected last line to contain footer status/hints, got %q", lastLine)
	}

	// 2. Open Group Editor with 30 paths while heavy logging is active
	home.editor.OpenCreate()
	for i := 1; i <= 30; i++ {
		home.editor.Paths = append(home.editor.Paths, fmt.Sprintf("/custom/path/number/%d", i))
	}

	// Add more logs
	for i := 101; i <= 200; i++ {
		home.console.AddLog("WARN", fmt.Sprintf("Heavy log entry %d during group editing with massive paths", i))
	}

	renderedWithEditor := home.Render()
	linesWithEditor := strings.Split(renderedWithEditor, "\n")
	if len(linesWithEditor) != 24 {
		t.Fatalf("expected Home.Render with editor to strictly equal 24 lines, got %d", len(linesWithEditor))
	}

	lastLineWithEditor := linesWithEditor[len(linesWithEditor)-1]
	if !strings.Contains(lastLineWithEditor, "STATUS") && !strings.Contains(lastLineWithEditor, "DOWN") {
		t.Fatalf("expected footer to be anchored on the last line during heavy logging + editor, got %q", lastLineWithEditor)
	}
}

func TestHomeProfileLaunchLoggingAndPersistence(t *testing.T) {
	home := &Home{}
	home.Init()

	if home.store == nil {
		t.Fatal("expected home.store to be initialized")
	}

	// Verify launch logs output reading from profile path
	foundReading := false
	foundLoaded := false
	expectedPath := home.store.Path

	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "Reading profiles from") && strings.Contains(entry.Message, expectedPath) {
			foundReading = true
		}
		if strings.Contains(entry.Message, "Loaded") && strings.Contains(entry.Message, expectedPath) {
			foundLoaded = true
		}
	}

	if !foundReading {
		t.Fatalf("expected launch log output 'Reading profiles from %s', logs: %v", expectedPath, home.console.Logs)
	}
	if !foundLoaded {
		t.Fatalf("expected launch log output 'Loaded ... from %s', logs: %v", expectedPath, home.console.Logs)
	}
}

func TestHomeTrafficChartIntegration(t *testing.T) {
	home := &Home{}
	home.Init()
	home.Update(tea.WindowSizeMsg{Width: 80, Height: 28})

	// Send a traffic tick
	home.Update(TrafficTickMsg{})

	rendered := home.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 28 {
		t.Fatalf("expected 28 lines total, got %d", len(lines))
	}

	// Verify Traffic Monitor is present in rendered output
	hasTrafficHeader := false
	for _, l := range lines {
		if strings.Contains(l, "Traffic Monitor") {
			hasTrafficHeader = true
			break
		}
	}
	if !hasTrafficHeader {
		t.Fatalf("expected rendered output to contain 'Traffic Monitor', got:\n%s", rendered)
	}

	t.Logf("Rendered Screen (Height=%d):\n%s", len(lines), rendered)
}

func TestHomeWarpCliLaunchDetection(t *testing.T) {
	home := &Home{}
	home.Init()

	// 1. Simulate installed and connected warp-cli
	msgInstalled := WarpDetectedMsg{
		Installed: true,
		Path:      "/usr/bin/warp-cli",
		Version:   "2026.7.1377.0",
		Status: warp.StatusInfo{
			Status:    "CONNECTED",
			Protocol:  "MASQUE",
			Mode:      "WarpProxy",
			ProxyPort: 40000,
		},
	}
	home.Update(msgInstalled)

	if home.networkCard.WarpStatus != "CONNECTED" {
		t.Errorf("expected networkCard.WarpStatus to be CONNECTED, got %s", home.networkCard.WarpStatus)
	}
	if home.networkCard.Protocol != "MASQUE" {
		t.Errorf("expected networkCard.Protocol to be MASQUE, got %s", home.networkCard.Protocol)
	}

	foundDetectedLog := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "Detected warp-cli at /usr/bin/warp-cli") {
			foundDetectedLog = true
			break
		}
	}
	if !foundDetectedLog {
		t.Fatalf("expected log confirming warp-cli detection, logs: %v", home.console.Logs)
	}

	// 2. Simulate uninstalled warp-cli
	msgUninstalled := WarpDetectedMsg{
		Installed: false,
	}
	home.Update(msgUninstalled)

	if home.networkCard.WarpStatus != "NOT INSTALLED" {
		t.Errorf("expected networkCard.WarpStatus to be NOT INSTALLED, got %s", home.networkCard.WarpStatus)
	}

	foundWarnLog := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "warp-cli is not installed") {
			foundWarnLog = true
			break
		}
	}
	if !foundWarnLog {
		t.Fatalf("expected warning log when warp-cli is uninstalled, logs: %v", home.console.Logs)
	}
}

func TestHomeClashInspectionLaunchDetection(t *testing.T) {
	home := &Home{}
	home.Init()

	// 1. Simulate active Verge and Mihomo
	msgInstalled := ClashDetectedMsg{
		Inspection: clash.Inspection{
			Installed:           true,
			ActiveProfileName:   "MockAirport",
			ActiveProfileUID:    "UID123",
			SocketAvailable:     true,
			SocketPath:          "/tmp/verge/verge-mihomo.sock",
			MihomoVersion:       "v1.19.29",
			HasWarpLocalProxy:   true,
			HasWarpSvcGuardRule: true,
			WarpRulesCount:      6,
			SummaryStatus:       "SYNCED (6 rules)",
		},
	}
	home.Update(msgInstalled)

	if home.networkCard.MihomoRule != "SYNCED (6 rules)" {
		t.Errorf("expected networkCard.MihomoRule to be 'SYNCED (6 rules)', got %s", home.networkCard.MihomoRule)
	}

	foundVergeLog := false
	foundSocketLog := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "Detected Clash Verge Rev (active profile: MockAirport)") {
			foundVergeLog = true
		}
		if strings.Contains(entry.Message, "Mihomo core v1.19.29 active") {
			foundSocketLog = true
		}
	}
	if !foundVergeLog {
		t.Fatalf("expected log confirming Clash Verge active profile, logs: %v", home.console.Logs)
	}
	if !foundSocketLog {
		t.Fatalf("expected log confirming Mihomo core socket active, logs: %v", home.console.Logs)
	}

	// 2. Simulate uninstalled Verge
	msgUninstalled := ClashDetectedMsg{
		Inspection: clash.Inspection{
			Installed: false,
		},
	}
	home.Update(msgUninstalled)

	if home.networkCard.MihomoRule != "NOT INSTALLED" {
		t.Errorf("expected networkCard.MihomoRule to be 'NOT INSTALLED', got %s", home.networkCard.MihomoRule)
	}
}

func TestHomeNetworkCardRefreshAndStatusChangeLogging(t *testing.T) {
	home := &Home{}
	home.Init()

	// 1. Test Refresh trigger via RefreshNetworkMsg
	_, cmd := home.Update(components.RefreshNetworkMsg{})
	if cmd == nil {
		t.Fatalf("expected non-nil cmd on RefreshNetworkMsg")
	}

	foundRefreshLog := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "Refreshing network & tunnel status...") {
			foundRefreshLog = true
			break
		}
	}
	if !foundRefreshLog {
		t.Fatalf("expected log 'Refreshing network & tunnel status...' on refresh trigger, logs: %v", home.console.Logs)
	}

	// 2. Test status change logging on WarpDetectedMsg
	home.Update(WarpDetectedMsg{
		Installed: true,
		Status: warp.StatusInfo{
			Status:   "CONNECTED",
			Protocol: "MASQUE",
		},
	})
	home.Update(WarpDetectedMsg{
		Installed: true,
		Status: warp.StatusInfo{
			Status:   "DISCONNECTED",
			Protocol: "WireGuard",
		},
	})

	foundWarpStatusChange := false
	foundProtocolChange := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "WARP tunnel status changed: CONNECTED -> DISCONNECTED") {
			foundWarpStatusChange = true
		}
		if strings.Contains(entry.Message, "WARP protocol changed: MASQUE -> WireGuard") {
			foundProtocolChange = true
		}
	}
	if !foundWarpStatusChange {
		t.Errorf("expected log for WARP status change, logs: %v", home.console.Logs)
	}
	if !foundProtocolChange {
		t.Errorf("expected log for WARP protocol change, logs: %v", home.console.Logs)
	}

	// 3. Test status change logging on ClashDetectedMsg
	home.Update(ClashDetectedMsg{
		Inspection: clash.Inspection{
			Installed:     true,
			SummaryStatus: "SYNCED (6 rules)",
		},
	})
	home.Update(ClashDetectedMsg{
		Inspection: clash.Inspection{
			Installed:     true,
			SummaryStatus: "DESYNCED",
		},
	})

	foundClashChange := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "Mihomo rule status changed: SYNCED (6 rules) -> DESYNCED") {
			foundClashChange = true
			break
		}
	}
	if !foundClashChange {
		t.Errorf("expected log for Mihomo rule status change, logs: %v", home.console.Logs)
	}

	// 4. Test status change logging on TraceResultMsg
	home.Update(TraceResultMsg{
		Verification: &checker.ExitVerification{
			IP:      "104.28.195.192",
			Loc:     "US",
			Colo:    "LAX",
			Latency: 35 * time.Millisecond,
			IsWarp:  true,
		},
	})
	home.Update(TraceResultMsg{
		Verification: &checker.ExitVerification{
			IP:      "104.28.200.5",
			Loc:     "SG",
			Colo:    "SIN",
			Latency: 50 * time.Millisecond,
			IsWarp:  true,
		},
	})

	foundExitChange := false
	foundCountryChange := false
	foundColoChange := false
	for _, entry := range home.console.Logs {
		if strings.Contains(entry.Message, "WARP exit IP changed: 104.28.195.192 -> 104.28.200.5") {
			foundExitChange = true
		}
		if strings.Contains(entry.Message, "WARP exit country changed: US -> SG") {
			foundCountryChange = true
		}
		if strings.Contains(entry.Message, "WARP colo changed: LAX -> SIN") {
			foundColoChange = true
		}
	}
	if !foundExitChange {
		t.Errorf("expected log for exit IP change, logs: %v", home.console.Logs)
	}
	if !foundCountryChange {
		t.Errorf("expected log for country change, logs: %v", home.console.Logs)
	}
	if !foundColoChange {
		t.Errorf("expected log for colo change, logs: %v", home.console.Logs)
	}
}

func TestHomeSpinnerTickAndCompletion(t *testing.T) {
	home := &Home{}
	home.Init()

	if !home.refreshingNetwork {
		t.Fatal("expected refreshingNetwork to be true after Init")
	}
	if !home.networkCard.Refreshing {
		t.Fatal("expected networkCard.Refreshing to be true after Init")
	}
	if !home.console.Active {
		t.Fatal("expected console.Active to be true after Init")
	}
	if home.footer.State != "DOWN" {
		t.Fatalf("expected footer.State to be 'DOWN' after Init, got %s", home.footer.State)
	}

	// 1. Advance spinner with TickMsg
	tickMsg := home.spinner.Tick()
	_, tickCmd := home.Update(tickMsg)
	if tickCmd == nil {
		t.Fatal("expected non-nil cmd from spinner.TickMsg while refreshing")
	}
	if home.networkCard.SpinnerView == "" {
		t.Fatal("expected networkCard.SpinnerView to be populated by tick")
	}
	if home.console.SpinnerView == "" {
		t.Fatal("expected console.SpinnerView to be populated by tick")
	}

	// 2. Deliver all pending detection messages to complete checks
	home.Update(WarpDetectedMsg{
		Installed: true,
		Status: warp.StatusInfo{
			Status:   "CONNECTED",
			Protocol: "MASQUE",
		},
	})
	home.Update(ClashDetectedMsg{
		Inspection: clash.Inspection{
			Installed:     true,
			SummaryStatus: "SYNCED (6 rules)",
		},
	})
	home.Update(TraceResultMsg{
		Verification: &checker.ExitVerification{
			IP:      "104.28.195.192",
			Loc:     "US",
			Colo:    "LAX",
			Latency: 35 * time.Millisecond,
			IsWarp:  true,
		},
	})

	if home.refreshingNetwork {
		t.Fatal("expected refreshingNetwork to be false after all messages delivered")
	}
	if home.networkCard.Refreshing {
		t.Fatal("expected networkCard.Refreshing to be false after completion")
	}
	if home.console.Active {
		t.Fatal("expected console.Active to be false after completion")
	}
	if home.footer.State != "DOWN" {
		t.Fatalf("expected footer.State to remain DOWN when only WARP is connected, got %s", home.footer.State)
	}

	// 3. Ticking after completion returns nil cmd (stops animation loop)
	_, idleCmd := home.Update(tickMsg)
	if idleCmd != nil {
		t.Fatal("expected nil cmd from TickMsg after refreshing is complete")
	}
	if home.networkCard.SpinnerView != "" {
		t.Fatalf("expected networkCard.SpinnerView to be empty after completion, got %q", home.networkCard.SpinnerView)
	}
	if home.console.SpinnerView != "" {
		t.Fatalf("expected console.SpinnerView to be empty after completion, got %q", home.console.SpinnerView)
	}
}

func TestHomeDynamicInjectionAndColoredFooterHints(t *testing.T) {
	home := &Home{}
	home.Init()

	// Configure mock profiles: Profile 0 is ON (Antigravity), Profile 1 is OFF (Chrome)
	home.processList.Profiles = []process.Profile{
		process.NewProfile("Antigravity", process.MatchProcessName, "agy", true),
		process.NewProfile("Google Chrome", process.MatchProcessName, "chrome", false),
	}
	home.processList.Cursor = 0
	home.focusIndex = 0
	home.applyFocus()

	// 1. Process List focus: Profile 0 is ON -> hint should be "space: off" in red (styles.ColorDanger)
	foundSpace := false
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			foundSpace = true
			if hint.Text != "space: off" {
				t.Fatalf("expected hint 'space: off' for enabled profile, got %q", hint.Text)
			}
			if hint.Color != styles.ColorDanger {
				t.Fatalf("expected red ColorDanger for 'space: off', got %v", hint.Color)
			}
		}
	}
	if !foundSpace {
		t.Fatal("expected space hint in footer")
	}

	// 2. Move cursor down to Profile 1 (OFF) -> hint should immediately change to "space: on" in green
	home.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if home.processList.Cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", home.processList.Cursor)
	}
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			if hint.Text != "space: on" {
				t.Fatalf("expected hint 'space: on' for disabled profile, got %q", hint.Text)
			}
			if hint.Color != styles.ColorSuccess {
				t.Fatalf("expected green ColorSuccess for 'space: on', got %v", hint.Color)
			}
		}
	}

	// 3. Toggle Profile 1 with space -> should become ON and hint switches to "space: off" (red)
	_, cmdToggle := home.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if cmdToggle != nil {
		if msg := cmdToggle(); msg != nil {
			home.Update(msg)
		}
	}
	if !home.processList.Profiles[1].Enabled {
		t.Fatal("expected Profile 1 to be enabled after space toggle")
	}
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			if hint.Text != "space: off" {
				t.Fatalf("expected hint 'space: off' after toggling ON, got %q", hint.Text)
			}
			if hint.Color != styles.ColorDanger {
				t.Fatalf("expected red ColorDanger for 'space: off', got %v", hint.Color)
			}
		}
	}

	// 4. Tab to Network Card (focusIndex = 1) -> initially tunnel is OFF -> hint should be "space: start" (green)
	home.Update(tea.KeyPressMsg{Code: tea.KeyTab, Text: "\t"})
	if home.focusIndex != 1 {
		t.Fatalf("expected focusIndex=1, got %d", home.focusIndex)
	}
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			if hint.Text != "space: start" {
				t.Fatalf("expected hint 'space: start' when tunnel is inactive, got %q", hint.Text)
			}
			if hint.Color != styles.ColorSuccess {
				t.Fatalf("expected green ColorSuccess for 'space: start', got %v", hint.Color)
			}
		}
	}

	// 5. Simulate space to start tunnel -> TunnelToggledMsg{Active: true}
	home.Update(TunnelToggledMsg{Active: true, InjectedRules: 2, EnabledGroups: 2})
	if !home.tunnelActive {
		t.Fatal("expected tunnelActive=true")
	}
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			if hint.Text != "space: stop" {
				t.Fatalf("expected hint 'space: stop' when tunnel is active, got %q", hint.Text)
			}
			if hint.Color != styles.ColorDanger {
				t.Fatalf("expected red ColorDanger for 'space: stop', got %v", hint.Color)
			}
		}
	}

	// 6. Simulate space to stop tunnel -> TunnelToggledMsg{Active: false}
	home.Update(TunnelToggledMsg{Active: false})
	if home.tunnelActive {
		t.Fatal("expected tunnelActive=false")
	}
	for _, hint := range home.footer.Hints {
		if strings.Contains(hint.Text, "space:") {
			if hint.Text != "space: start" {
				t.Fatalf("expected hint 'space: start' after stopping tunnel, got %q", hint.Text)
			}
			if hint.Color != styles.ColorSuccess {
				t.Fatalf("expected green ColorSuccess for 'space: start', got %v", hint.Color)
			}
		}
	}

	// 7. Closing the UI must leave an explicitly started tunnel active.
	home.tunnelActive = true
	home.Cleanup()
	if !home.tunnelActive {
		t.Fatal("expected tunnelActive=true after Cleanup")
	}
}

func TestHomeLocksProfileEditsWhileTunnelActive(t *testing.T) {
	home := &Home{}
	home.Init()
	home.processList.Profiles = []process.Profile{process.NewProfile("App", process.MatchProcessName, "app", true)}
	home.processList.Cursor = 0
	home.focusIndex = 0
	home.tunnelActive = true
	home.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if !home.processList.Profiles[0].Enabled {
		t.Fatal("space changed process intent while tunnel was active")
	}
	home.Update(components.OpenGroupCreateMsg{})
	if home.editor.Open {
		t.Fatal("editor opened while tunnel was active")
	}
}
