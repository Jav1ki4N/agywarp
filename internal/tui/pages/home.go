package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	"agywarp/internal/checker"
	"agywarp/internal/clash"
	"agywarp/internal/process"
	"agywarp/internal/proxymode"
	"agywarp/internal/traffic"
	"agywarp/internal/tui/components"
	"agywarp/internal/tui/styles"
	"agywarp/internal/warp"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type ScannedProcessesMsg []process.RunningProcess
type TrafficTickMsg time.Time

// WarpDetectedMsg is delivered upon launch when warp-cli detection completes.
type WarpDetectedMsg struct {
	Installed bool
	Path      string
	Version   string
	Status    warp.StatusInfo
	Err       error
}

// ClashDetectedMsg is delivered upon launch when Clash Verge / Mihomo inspection completes.
type ClashDetectedMsg struct {
	Inspection clash.Inspection
	Route      *clash.OuterRoute
	Err        error
}

// TraceResultMsg is delivered when outbound WARP exit verification completes.
type TraceResultMsg struct {
	Verification *checker.ExitVerification
	Err          error
}

// TunnelToggledMsg is emitted when dynamic injection/WARP tunnel state changes.
type TunnelToggledMsg struct {
	Route         *clash.OuterRoute
	Active        bool
	EnabledGroups int
	InjectedRules int
	Err           error
}

type CurrentNodeTestedMsg struct {
	Route        *clash.OuterRoute
	Verification *checker.ExitVerification
	Err          error
}

type RouteGuardTickMsg struct {
	Route  *clash.OuterRoute
	Change string
	Err    error
}

type Home struct {
	warpAnnounced  bool
	clashAnnounced bool
	proxyMode      proxymode.Mode
	settingsPath   string
	PageBase
	InitialTunnelActive bool
	processList         components.ProcessList
	networkCard         components.NetworkCard
	trafficChart        components.TrafficChart
	editor              components.GroupEditor
	picker              components.ProcessPicker
	console             components.Console
	outputExpanded      bool
	footer              components.Footer
	scanner             process.Scanner
	store               *process.Store
	trafficMonitor      traffic.Monitor
	warpClient          warp.Client
	clashManager        clash.Manager
	checker             checker.Checker
	spinner             spinner.Model
	refreshingNetwork   bool
	pendingChecks       int
	tunnelActive        bool
	tunnelBusy          bool
	focusIndex          int // 0: ProcessList, 1: NetworkCard, 2: Console
	lastRunning         []process.RunningProcess
	routeGuardTriggered bool
	routeGuardError     string
}

func (h *Home) Init() tea.Cmd {
	h.tunnelActive = h.InitialTunnelActive
	h.processList = components.NewProcessList()
	h.networkCard = components.NewNetworkCard()
	h.editor = components.NewGroupEditor()
	h.picker = components.NewProcessPicker()
	h.trafficChart = components.NewTrafficChart()
	h.trafficMonitor = traffic.NewLinuxMonitor()
	h.warpClient = warp.NewClient()
	h.clashManager = clash.NewManager()
	h.checker = checker.NewChecker()
	h.console = components.NewConsole()
	h.initProxyMode()
	h.footer = components.NewFooter()
	h.scanner = process.NewScanner()
	h.focusIndex = 0

	// Initialize spinner model for in-progress operations (displayed only at log)
	h.spinner = spinner.New(
		spinner.WithSpinner(spinner.Line),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(styles.ColorWarning)),
	)
	h.refreshingNetwork = true
	h.pendingChecks = 3
	h.networkCard.Refreshing = true
	h.networkCard.SpinnerView = h.spinner.View()
	h.console.Active = true
	h.console.SpinnerView = h.spinner.View()

	// Take initial baseline traffic sample
	if sample, err := h.trafficMonitor.Sample(); err == nil {
		h.trafficChart.AddSample(sample)
	}

	// Initialize profile storage and load persistent profiles
	store, err := process.NewStore()
	if err == nil {
		h.store = store
		loaded, loadErr := store.Load()
		if loadErr == nil {
			h.processList.Profiles = loaded
			h.console.AddLog("OK", fmt.Sprintf("Loaded %d profile(s) from %s", len(loaded), store.Path))
		} else {
			h.console.AddLog("ERR", fmt.Sprintf("Failed to load profiles from %s: %v", store.Path, loadErr))
		}
	} else {
		h.console.AddLog("ERR", fmt.Sprintf("Failed to initialize profile store: %v", err))
	}

	h.applyFocus()
	if h.tunnelActive {
		h.networkCard.ServiceStatus = "ACTIVE [ON]"
	}

	return tea.Batch(
		h.spinner.Tick,
		h.processList.Init(),
		h.networkCard.Init(),
		h.trafficChart.Init(),
		h.editor.Init(),
		h.picker.Init(),
		h.console.Init(),
		h.footer.Init(),
		h.triggerScanCmd(),
		h.tickTrafficCmd(),
		h.routeGuardTickCmd(),
		h.detectWarpCmd(),
		h.detectClashCmd(),
		h.checkTraceCmd(),
	)
}

func (h *Home) routeGuardTickCmd() tea.Cmd {
	active, busy, manager := h.tunnelActive, h.tunnelBusy, h.clashManager
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		if busy || manager == nil {
			return RouteGuardTickMsg{}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		route := inspectOuterRoute(ctx, manager)
		if !active {
			return RouteGuardTickMsg{Route: route}
		}
		change, err := manager.RuntimeRouteChanged(ctx)
		return RouteGuardTickMsg{Route: route, Change: change, Err: err}
	})
}

func (h *Home) tickTrafficCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return TrafficTickMsg(t)
	})
}

func (h *Home) detectWarpCmd() tea.Cmd {
	return func() tea.Msg {
		if h.warpClient == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		install, err := h.warpClient.Detect(ctx)
		if err != nil || !install.Installed {
			return WarpDetectedMsg{Installed: false, Err: err}
		}

		status, _ := h.warpClient.Status(ctx)
		return WarpDetectedMsg{
			Installed: true,
			Path:      install.Path,
			Version:   install.Version,
			Status:    status,
		}
	}
}

func (h *Home) detectClashCmd() tea.Cmd {
	return func() tea.Msg {
		if h.clashManager == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		insp, err := h.clashManager.Inspect(ctx)
		return ClashDetectedMsg{Inspection: insp, Route: inspectOuterRoute(ctx, h.clashManager), Err: err}
	}
}

func (h *Home) checkTraceCmd() tea.Cmd {
	return func() tea.Msg {
		if h.checker == nil {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status, err := h.warpClient.Status(ctx)
		if err != nil {
			return TraceResultMsg{Err: err}
		}
		if status.Mode != "WarpProxy" || status.ProxyPort < 1 {
			return TraceResultMsg{Err: fmt.Errorf("WARP local proxy is not configured")}
		}
		verif, err := h.checker.VerifyExit(ctx, fmt.Sprintf("127.0.0.1:%d", status.ProxyPort))
		return TraceResultMsg{Verification: verif, Err: err}
	}
}

func (h *Home) saveProfiles() {
	if h.store == nil {
		return
	}
	if err := h.store.Save(h.processList.Profiles); err != nil {
		h.console.AddLog("ERR", fmt.Sprintf("Failed to save profiles to %s: %v", h.store.Path, err))
	}
}

func (h *Home) triggerScanCmd() tea.Cmd {
	return func() tea.Msg {
		if h.scanner == nil {
			return nil
		}
		running, err := h.scanner.Scan(context.Background())
		if err != nil {
			return nil
		}
		return ScannedProcessesMsg(running)
	}
}

func (h *Home) applyFocus() {
	h.processList.Focused = (h.focusIndex == 0)
	h.networkCard.Focused = (h.focusIndex == 1)
	h.console.Focused = (h.focusIndex == 2)

	switch h.focusIndex {
	case 0:
		if h.tunnelActive || h.tunnelBusy {
			h.footer.Hints = []components.FooterHint{
				components.Hint("tab: next block"),
				components.Hint("groups locked while tunnel is on", styles.ColorWarning),
				components.Hint("q: quit"),
			}
			return
		}
		spaceHint := components.Hint("space: on", styles.ColorSuccess)
		if sel := h.processList.Selected(); sel != nil && sel.Enabled {
			spaceHint = components.Hint("space: off", styles.ColorDanger)
		}
		h.footer.Hints = []components.FooterHint{
			components.Hint("tab: next block"),
			components.Hint("a: add group"),
			components.Hint("e: edit"),
			spaceHint,
			components.Hint("d: del"),
			components.Hint("q: quit"),
		}
	case 1:
		spaceHint := components.Hint("space: start", styles.ColorSuccess)
		if h.tunnelActive {
			spaceHint = components.Hint("space: stop", styles.ColorDanger)
		}
		h.footer.Hints = []components.FooterHint{
			components.Hint("tab: next block"),
			spaceHint,
			components.Hint("t: test node"),
			components.Hint("p: proxy mode"),
			components.Hint("r: refresh"),
			components.Hint("q: quit"),
		}
		if h.tunnelActive {
			h.footer.Hints[2] = components.Hint("OFF before switching airport/node", styles.ColorWarning)
		}
	case 2:
		h.footer.Hints = []components.FooterHint{
			components.Hint("tab: next block"),
			components.Hint("o: expand"),
			components.Hint("PgUp/PgDn: scroll"),
			components.Hint("End: latest"),
			components.Hint("q: quit"),
		}
	}
}

// testCurrentNode uses the current Mihomo warp-svc route and restores the
// bootstrap config and WARP connection state before reporting its result.
func testCurrentNode(ctx context.Context, manager clash.Manager, client warp.Client, verifier checker.Checker) (result *checker.ExitVerification, route *clash.OuterRoute, err error) {
	status, err := client.Status(ctx)
	if err != nil {
		return nil, route, fmt.Errorf("WARP status: %w", err)
	}
	if status.Mode != "WarpProxy" || status.ProxyPort < 1 {
		return nil, route, fmt.Errorf("WARP must use WarpProxy mode with a valid port")
	}
	if status.Status == "CONNECTING" || status.Status == "DISCONNECTING" {
		return nil, route, fmt.Errorf("WARP is %s; retry when its state settles", status.Status)
	}
	check, err := manager.Preflight(ctx)
	if err != nil {
		return nil, route, fmt.Errorf("Mihomo preflight: %w", err)
	}
	if check.Active {
		return nil, route, fmt.Errorf("agywarp runtime is already active")
	}
	connectedByUs := status.Status != "CONNECTED"
	connectAttempted := false
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		if restoreErr := manager.AbortBootstrap(restoreCtx); restoreErr != nil {
			if err == nil {
				err = fmt.Errorf("restore failed: %w; WARP left connected", restoreErr)
			} else {
				err = fmt.Errorf("%v; restore failed: %w; WARP left connected", err, restoreErr)
			}
			return
		}
		if connectAttempted {
			if disconnectErr := client.Disconnect(restoreCtx); disconnectErr != nil {
				if err == nil {
					err = fmt.Errorf("WARP disconnect failed: %w", disconnectErr)
				} else {
					err = fmt.Errorf("%v; WARP disconnect failed: %w", err, disconnectErr)
				}
			}
		}
	}()
	if err := manager.BootstrapRuntime(ctx, status.ProxyPort); err != nil {
		return nil, route, fmt.Errorf("load bootstrap config: %w", err)
	}
	if connectedByUs {
		connectAttempted = true
		if err := client.Connect(ctx); err != nil {
			return nil, route, fmt.Errorf("connect WARP: %w", err)
		}
	}
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err = waitForWarpProxy(readyCtx, client, verifier, status.ProxyPort)
	cancel()
	if err != nil {
		return nil, route, err
	}
	result, err = verifier.VerifyExit(ctx, fmt.Sprintf("127.0.0.1:%d", status.ProxyPort))
	if err != nil {
		err = fmt.Errorf("WARP exit verification: %w", err)
		// Keep manual API diagnostics independent of trace success.
		result = &checker.ExitVerification{}
	} else if result == nil || !result.IsWarp {
		err = fmt.Errorf("WARP exit verification did not confirm WARP")
	}
	if result == nil {
		result = &checker.ExitVerification{}
	}

	if prober, ok := verifier.(checker.APIProber); ok {
		result.APIProbes = prober.ProbeAPIs(ctx, fmt.Sprintf("127.0.0.1:%d", status.ProxyPort))
	}
	route = observeOuterRoute(ctx, manager)
	return result, route, err
}

func (h *Home) testCurrentNodeCmd() tea.Cmd {
	if h.tunnelActive || h.tunnelBusy {
		h.console.AddLog("WARN", "Stop the tunnel before testing the current node")
		return nil
	}
	h.tunnelBusy = true
	h.applyFocus()
	h.console.AddLog("INFO", "Testing WARP through the current Mihomo outer route ("+h.proxyMode.Label()+")...")
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		verification, route, err := testCurrentNode(ctx, h.clashManager, h.warpClient, h.checker)
		return CurrentNodeTestedMsg{Verification: verification, Route: route, Err: err}
	}
}

func waitForWarpProxy(ctx context.Context, client warp.Client, verifier checker.Checker, port int) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var last warp.StatusInfo
	var lastErr error
	for {
		status, err := client.Status(ctx)
		if err == nil {
			last = status
			lastErr = nil
			switch status.Status {
			case "CONNECTED":
				if verifier.CheckPortListening(ctx, addr) {
					return nil
				}
			case "UNABLE", "ERROR":
				if status.Reason != "" {
					hint := ""
					if strings.Contains(strings.ToLower(status.Reason), "happy eyeballs") {
						hint = "; check UDP connectivity to the WARP edge through the selected outer route"
					}
					return fmt.Errorf("WARP connection failed (%s): %s%s", status.Status, status.Reason, hint)
				}
				return fmt.Errorf("WARP connection failed: %s", status.Status)
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if last.Status == "CONNECTED" {
				return fmt.Errorf("WARP connected but proxy %s is not listening: %w", addr, ctx.Err())
			}
			if lastErr != nil {
				return fmt.Errorf("WARP status check failed: %w", lastErr)
			}
			if last.Reason != "" {
				return fmt.Errorf("WARP proxy %s not ready (status %s: %s): %w", addr, last.Status, last.Reason, ctx.Err())
			}
			return fmt.Errorf("WARP proxy %s not ready (status %s): %w", addr, last.Status, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (h *Home) toggleTunnelCmd() tea.Cmd {
	if h.tunnelBusy {
		h.console.AddLog("WARN", "Tunnel operation already in progress")
		return nil
	}
	h.tunnelBusy = true
	if !h.tunnelActive {
		h.console.AddLog("INFO", "Starting WARP and checking its local proxy ("+h.proxyMode.Label()+")...")
		var rules []string
		enabledCount := 0
		for _, p := range h.processList.Profiles {
			if p.Enabled {
				enabledCount++
				rules = append(rules, p.ToProcessRules("AGYWARP-WARP")...)
			}
		}
		if len(rules) == 0 {
			h.tunnelBusy = false
			h.console.AddLog("WARN", "Enable at least one process group before starting the tunnel")
			return nil
		}
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
			defer cancel()
			status, err := h.warpClient.Status(ctx)
			if err != nil {
				return TunnelToggledMsg{Err: fmt.Errorf("WARP status: %w", err)}
			}
			if status.Mode != "WarpProxy" || status.ProxyPort < 1 {
				return TunnelToggledMsg{Err: fmt.Errorf("WARP must use WarpProxy mode with a valid port")}
			}
			connectedByUs := status.Status != "CONNECTED"
			if err := h.clashManager.BootstrapRuntime(ctx, status.ProxyPort); err != nil {
				restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer restoreCancel()
				if restoreErr := h.clashManager.AbortBootstrap(restoreCtx); restoreErr != nil {
					return TunnelToggledMsg{Err: fmt.Errorf("load bootstrap config: %w; restore failed: %v", err, restoreErr)}
				}
				return TunnelToggledMsg{Err: fmt.Errorf("load bootstrap config: %w", err)}
			}
			rollback := func(cause error) tea.Msg {
				restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer restoreCancel()
				if err := h.clashManager.AbortBootstrap(restoreCtx); err != nil {
					return TunnelToggledMsg{Err: fmt.Errorf("%v; restore failed: %w; WARP left connected", cause, err)}
				}
				if connectedByUs {
					if err := h.warpClient.Disconnect(restoreCtx); err != nil {
						return TunnelToggledMsg{Err: fmt.Errorf("%v; WARP disconnect failed: %w", cause, err)}
					}
				}
				return TunnelToggledMsg{Err: cause}
			}
			if connectedByUs {
				if err := h.warpClient.Connect(ctx); err != nil {
					return rollback(fmt.Errorf("connect WARP: %w", err))
				}
			}
			readyCtx, readyCancel := context.WithTimeout(ctx, 30*time.Second)
			readyErr := waitForWarpProxy(readyCtx, h.warpClient, h.checker, status.ProxyPort)
			readyCancel()
			if readyErr != nil {
				return rollback(readyErr)
			}
			verification, err := h.checker.VerifyExit(ctx, fmt.Sprintf("127.0.0.1:%d", status.ProxyPort))
			if err != nil || verification == nil || !verification.IsWarp {
				return rollback(fmt.Errorf("WARP exit verification failed: %v", err))
			}
			route := observeOuterRoute(ctx, h.clashManager)
			count, err := h.clashManager.StartRuntime(ctx, rules, status.ProxyPort, connectedByUs)
			if err != nil {
				return rollback(fmt.Errorf("load runtime config: %w", err))
			}
			return TunnelToggledMsg{Active: true, EnabledGroups: enabledCount, InjectedRules: count, Route: route}
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		connectedByUs, err := h.clashManager.StopRuntime(ctx)
		if err != nil {
			return TunnelToggledMsg{Active: true, Err: fmt.Errorf("restore base config: %w", err)}
		}
		if connectedByUs {
			if err := h.warpClient.Disconnect(ctx); err != nil {
				return TunnelToggledMsg{Active: false, Err: fmt.Errorf("base restored; disconnect WARP: %w", err)}
			}
		}
		return TunnelToggledMsg{Active: false}
	}
}

// Closing the UI leaves the explicitly started tunnel running.
func (h *Home) Cleanup() {}

func (h *Home) cycleFocus(delta int) {
	totalBlocks := 3
	h.focusIndex = (h.focusIndex + delta + totalBlocks) % totalBlocks
	h.applyFocus()
}

func (h *Home) updateFooterStatus() {
	if h.refreshingNetwork {
		h.footer.State = "REFRESHING"
		h.footer.StateBg = styles.ColorWarning
	} else if h.tunnelActive && h.networkCard.WarpStatus == "CONNECTED" && h.networkCard.ExitIP != "---" && strings.HasPrefix(h.networkCard.MihomoRule, "SYNCED") {
		h.footer.State = "UP"
		h.footer.StateBg = styles.ColorSuccess
	} else {
		h.footer.State = "DOWN"
		h.footer.StateBg = styles.ColorDanger
	}
}

func (h *Home) triggerNetworkRefresh() tea.Cmd {
	h.refreshingNetwork = true
	h.pendingChecks = 3
	h.networkCard.Refreshing = true
	h.footer.Refreshing = true
	h.console.Active = true
	h.networkCard.SpinnerView = h.spinner.View()
	h.footer.SpinnerView = h.spinner.View()
	h.console.SpinnerView = h.spinner.View()
	h.updateFooterStatus()

	return tea.Batch(
		h.spinner.Tick,
		h.detectWarpCmd(),
		h.detectClashCmd(),
		h.checkTraceCmd(),
	)
}

func (h *Home) checkPendingDone() {
	h.pendingChecks--
	if h.pendingChecks <= 0 {
		h.pendingChecks = 0
		h.refreshingNetwork = false
		h.networkCard.Refreshing = false
		h.footer.Refreshing = false
		h.console.Active = false
		h.networkCard.SpinnerView = ""
		h.footer.SpinnerView = ""
		h.console.SpinnerView = ""
	}
	h.updateFooterStatus()
}

func (h *Home) Update(msg tea.Msg) (Page, tea.Cmd) {
	var cmds []tea.Cmd

	// 1. Group Editor modal captures input exclusively when open
	if h.editor.Open {
		switch msg := msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg:
			cmd := h.editor.Update(msg)
			return h, cmd
		case tea.PasteStartMsg, tea.PasteEndMsg:
			return h, nil
		}
	}

	// 2. Picker modal captures input exclusively when open
	if h.picker.Open {
		switch msg := msg.(type) {
		case tea.KeyPressMsg, tea.PasteMsg:
			cmd := h.picker.Update(msg)
			return h, cmd
		case tea.PasteStartMsg, tea.PasteEndMsg:
			return h, nil
		}
	}

	switch msg := msg.(type) {
	case spinner.TickMsg:
		if h.refreshingNetwork || h.pendingChecks > 0 || h.picker.Scanning {
			var cmd tea.Cmd
			h.spinner, cmd = h.spinner.Update(msg)
			h.networkCard.SpinnerView = h.spinner.View()
			h.footer.SpinnerView = h.spinner.View()
			h.picker.SpinnerView = h.spinner.View()
			return h, cmd
		}
		h.networkCard.Refreshing = false
		h.footer.Refreshing = false
		h.picker.Scanning = false
		h.networkCard.SpinnerView = ""
		h.footer.SpinnerView = ""
		h.picker.SpinnerView = ""
		h.updateFooterStatus()
		return h, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "o", "O":
			h.outputExpanded = !h.outputExpanded
			h.focusIndex = 2
			h.applyFocus()
			return h, nil
		case "pgup", "pgdown", "end":
			return h, h.console.Update(msg)
		}
		if h.outputExpanded && msg.String() != "q" && msg.String() != "ctrl+c" {
			return h, nil
		}
		if h.focusIndex == 0 && (h.tunnelActive || h.tunnelBusy) {
			switch msg.String() {
			case "a", "e", "d", "m", " ", "space":
				h.console.AddLog("WARN", "Process groups are locked while the network tunnel is ON")
				return h, nil
			}
		}
		switch msg.String() {
		case "tab", "\t":
			h.cycleFocus(1)
			return h, nil
		case "shift+tab", "backtab":
			h.cycleFocus(-1)
			return h, nil
		case " ", "space":
			if h.focusIndex == 1 {
				return h, h.toggleTunnelCmd()
			}
		case "r", "R":
			if h.focusIndex == 1 {
				return h, h.triggerNetworkRefresh()
			}
		case "p", "P":
			if h.focusIndex == 1 {
				h.switchProxyMode()
				return h, nil
			}
		case "t", "T":
			if h.focusIndex == 1 {
				return h, h.testCurrentNodeCmd()
			}
		case "q", "ctrl+c":
			if h.tunnelActive || h.tunnelBusy {
				h.console.AddLog("WARN", "Turn the tunnel OFF before quitting or switching airport or node in Clash Verge")
				return h, nil
			}
			return h, tea.Quit
		}

	case components.ToggleTunnelMsg:
		return h, h.toggleTunnelCmd()

	case CurrentNodeTestedMsg:
		h.showOuterRoute(msg.Route, true)
		h.tunnelBusy = false
		h.applyFocus()
		if msg.Err != nil {
			h.console.AddLog("ERR", fmt.Sprintf("WARP test or restoration failed: %v", msg.Err))
		}
		if msg.Verification != nil {
			if msg.Verification.IsWarp {
				h.console.AddLog("OK", fmt.Sprintf("WARP exit · %s · %s · %dms", msg.Verification.IP, msg.Verification.Loc, msg.Verification.Latency.Milliseconds()))
			}
			h.showAPIProbes(msg.Verification.APIProbes)
		}
		return h, h.triggerNetworkRefresh()

	case RouteGuardTickMsg:
		h.showOuterRoute(msg.Route, false)
		next := h.routeGuardTickCmd()
		if msg.Err != nil {
			if h.tunnelActive && h.routeGuardError != msg.Err.Error() {
				h.routeGuardError = msg.Err.Error()
				h.console.AddLog("WARN", fmt.Sprintf("Cannot verify airport/node selection: %v", msg.Err))
			}
			return h, next
		}
		h.routeGuardError = ""
		if msg.Change != "" && h.tunnelActive && !h.tunnelBusy && !h.routeGuardTriggered {
			h.routeGuardTriggered = true
			h.console.AddLog("ERR", fmt.Sprintf("%s while tunnel is ON; stopping WARP routing. Turn OFF before switching airport or node.", msg.Change))
			return h, tea.Batch(next, h.toggleTunnelCmd())
		}
		return h, next

	case TunnelToggledMsg:
		h.showOuterRoute(msg.Route, msg.Active)
		h.tunnelBusy = false
		if !msg.Active {
			h.routeGuardTriggered = false
		}
		if msg.Err != nil {
			h.tunnelActive = msg.Active
			h.applyFocus()
			if !msg.Active {
				h.networkCard.ServiceStatus = "INACTIVE [OFF]"
			}
			h.console.AddLog("ERR", msg.Err.Error())
			return h, nil
		}

		h.tunnelActive = msg.Active
		h.applyFocus()

		if msg.Active {
			h.networkCard.ServiceStatus = "ACTIVE [ON]"
			if msg.InjectedRules > 0 {
				h.console.AddLog("OK", fmt.Sprintf("Routing ON · %d groups · %d rules · %s", msg.EnabledGroups, msg.InjectedRules, h.proxyMode.Label()))
			} else {
				h.console.AddLog("WARN", "Routing ON · no process rules loaded")
			}
			h.console.AddLog("WARN", "Turn OFF before switching airport or node; keep this dashboard open while ON")
		} else {
			h.networkCard.ServiceStatus = "INACTIVE [OFF]"
			h.console.AddLog("OK", "Routing OFF · base restored · prior WARP state restored")
		}

		return h, h.triggerNetworkRefresh()

	case components.RefreshNetworkMsg:
		return h, h.triggerNetworkRefresh()

	case ScannedProcessesMsg:
		h.lastRunning = msg
		h.picker.Scanning = false
		h.picker.SpinnerView = ""
		h.picker.SetRunningProcesses(msg)
		h.editor.SetRunningProcesses(msg)

		// Map running counts to current profiles
		counts := make(map[string]int)
		for _, prof := range h.processList.Profiles {
			counts[prof.ID] = process.CountRunningInstances(prof, msg)
		}
		h.processList.RunningCounts = counts
		return h, nil

	case WarpDetectedMsg:
		if msg.Installed {
			verStr := msg.Version
			if verStr != "" && !strings.HasPrefix(verStr, "v") {
				verStr = "v" + verStr
			}

			prevWarpStatus := h.networkCard.WarpStatus
			prevProtocol := h.networkCard.Protocol

			newWarpStatus := msg.Status.Status
			if newWarpStatus == "" {
				newWarpStatus = "DISCONNECTED"
			}
			newProtocol := msg.Status.Protocol
			if newProtocol == "" {
				newProtocol = "---"
			}

			if !h.warpAnnounced {
				h.console.AddLog("OK", fmt.Sprintf("WARP %s · %s · %s · :%d", verStr, newWarpStatus, newProtocol, msg.Status.ProxyPort))
				h.warpAnnounced = true
			} else if prevWarpStatus != newWarpStatus || prevProtocol != newProtocol {
				h.console.AddLog("INFO", fmt.Sprintf("WARP status: %s -> %s · %s", prevWarpStatus, newWarpStatus, newProtocol))
			}

			h.networkCard.WarpStatus = newWarpStatus
			h.networkCard.Protocol = newProtocol

			if !h.refreshingNetwork && prevWarpStatus != "CONNECTED" && newWarpStatus == "CONNECTED" {
				h.pendingChecks++
				h.refreshingNetwork = true
				h.networkCard.Refreshing = true
				h.footer.Refreshing = true
				h.networkCard.SpinnerView = h.spinner.View()
				h.footer.SpinnerView = h.spinner.View()
				h.updateFooterStatus()
				return h, tea.Batch(h.spinner.Tick, h.checkTraceCmd())
			}
		} else {
			if h.networkCard.WarpStatus != "NOT INSTALLED" {
				h.console.AddLog("WARN", "warp-cli is not installed or not in PATH")
			}
			h.networkCard.WarpStatus = "NOT INSTALLED"
			h.warpAnnounced = false
		}
		h.checkPendingDone()
		return h, nil

	case ClashDetectedMsg:
		h.showOuterRoute(msg.Route, false)
		if msg.Inspection.Installed {
			subName := msg.Inspection.ActiveProfileName
			if subName == "" {
				subName = msg.Inspection.ActiveProfileUID
			}

			prevRuleStatus := h.networkCard.MihomoRule
			newRuleStatus := msg.Inspection.SummaryStatus

			if !h.clashAnnounced {
				if msg.Inspection.SocketAvailable {
					h.console.AddLog("OK", fmt.Sprintf("Mihomo %s · Source: %s", msg.Inspection.MihomoVersion, subName))
				} else {
					h.console.AddLog("WARN", fmt.Sprintf("Mihomo socket unavailable: %s · Source: %s", msg.Inspection.SocketPath, subName))
				}
				h.clashAnnounced = true
			} else if prevRuleStatus != newRuleStatus {
				h.console.AddLog("INFO", fmt.Sprintf("Mihomo rules: %s -> %s", prevRuleStatus, newRuleStatus))
			}

			h.networkCard.MihomoRule = newRuleStatus
		} else {
			if h.networkCard.MihomoRule != "NOT INSTALLED" {
				h.console.AddLog("WARN", "Clash Verge Rev directory not found")
			}
			h.networkCard.MihomoRule = "NOT INSTALLED"
			h.clashAnnounced = false
		}
		h.checkPendingDone()
		return h, nil

	case TraceResultMsg:
		if msg.Err == nil && msg.Verification != nil && msg.Verification.IsWarp {
			prevIP := h.networkCard.ExitIP
			prevCountry := h.networkCard.ExitCountry
			prevColo := h.networkCard.Colo

			newIP := msg.Verification.IP
			newCountry := msg.Verification.Loc
			newColo := msg.Verification.Colo
			latencyStr := fmt.Sprintf("%dms", msg.Verification.Latency.Milliseconds())

			if prevIP != newIP || prevCountry != newCountry || prevColo != newColo {
				label := "WARP exit"
				if prevIP != "" && prevIP != "---" {
					label = "WARP exit changed"
				}
				h.console.AddLog("INFO", fmt.Sprintf("%s · %s · %s · %s · %s", label, newIP, newCountry, newColo, latencyStr))
			}

			h.networkCard.ExitIP = newIP
			h.networkCard.ExitCountry = newCountry
			h.networkCard.Colo = newColo
			h.networkCard.Latency = latencyStr
		} else {
			if h.networkCard.ExitIP != "---" && h.networkCard.ExitIP != "" {
				h.console.AddLog("WARN", fmt.Sprintf("WARP exit trace unreachable: %v", msg.Err))
				h.networkCard.ExitIP = "---"
				h.networkCard.ExitCountry = "---"
				h.networkCard.Colo = "---"
				h.networkCard.Latency = "---"
			}
		}
		h.checkPendingDone()
		return h, nil

	case components.LogMsg:
		h.console.AddLog(msg.Level, msg.Message)
		return h, nil

	case components.OpenGroupCreateMsg:
		if h.tunnelActive || h.tunnelBusy {
			h.console.AddLog("WARN", "Process groups are locked while the network tunnel is ON")
			return h, nil
		}
		h.editor.SetRunningProcesses(h.lastRunning)
		h.editor.OpenCreate()
		return h, nil

	case components.OpenGroupEditMsg:
		if h.tunnelActive || h.tunnelBusy {
			h.console.AddLog("WARN", "Process groups are locked while the network tunnel is ON")
			return h, nil
		}
		h.editor.SetRunningProcesses(h.lastRunning)
		h.editor.OpenEdit(msg.Profile)
		return h, nil

	case components.GroupSavedMsg:
		if h.tunnelActive || h.tunnelBusy {
			h.console.AddLog("WARN", "Process groups are locked while the network tunnel is ON")
			return h, nil
		}
		if msg.IsEditing {
			// Update existing group in place
			for i, p := range h.processList.Profiles {
				if p.ID == msg.Profile.ID {
					h.processList.Profiles[i] = msg.Profile
					break
				}
			}
			h.processList.RunningCounts[msg.Profile.ID] = process.CountRunningInstances(msg.Profile, h.lastRunning)
			h.console.AddLog("OK", fmt.Sprintf("Updated process group: %s (%d paths)", msg.Profile.Label, len(msg.Profile.Matchers)))
		} else {
			// Add new group
			h.processList.Profiles = append(h.processList.Profiles, msg.Profile)
			if len(h.processList.Profiles) == 1 {
				h.processList.Cursor = 0
			}
			h.processList.RunningCounts[msg.Profile.ID] = process.CountRunningInstances(msg.Profile, h.lastRunning)
			h.console.AddLog("OK", fmt.Sprintf("Created process group: %s (%d paths)", msg.Profile.Label, len(msg.Profile.Matchers)))
		}
		h.saveProfiles()
		h.applyFocus()
		return h, nil

	case components.OpenProcessPickerMsg:
		h.picker.Open = true
		h.picker.Scanning = true
		h.picker.SpinnerView = h.spinner.View()
		h.picker.SetRunningProcesses(h.lastRunning)
		return h, tea.Batch(h.spinner.Tick, h.triggerScanCmd())

	case components.ProfileAddedMsg:
		h.processList.Profiles = append(h.processList.Profiles, msg.Profile)
		if len(h.processList.Profiles) == 1 {
			h.processList.Cursor = 0
		}
		h.processList.RunningCounts[msg.Profile.ID] = process.CountRunningInstances(msg.Profile, h.lastRunning)
		pattern := msg.Profile.Label
		if len(msg.Profile.Matchers) > 1 {
			pattern = fmt.Sprintf("%d paths", len(msg.Profile.Matchers))
		} else if len(msg.Profile.Matchers) == 1 {
			pattern = msg.Profile.Matchers[0].Pattern
		}
		h.console.AddLog("OK", fmt.Sprintf("Added process group: %s (%s)", msg.Profile.Label, pattern))
		h.saveProfiles()
		h.applyFocus()
		return h, nil

	case components.ProcessToggledMsg:
		state := "OFF"
		if msg.Profile.Enabled {
			state = "ON"
		}
		h.console.AddLog("INFO", fmt.Sprintf("Routing intent toggled: %s -> %s", msg.Profile.Label, state))
		h.saveProfiles()
		h.applyFocus()

		return h, nil

	case components.ProcessDeletedMsg:
		h.console.AddLog("WARN", fmt.Sprintf("Removed process profile: %s", msg.Profile.Label))
		h.saveProfiles()
		h.applyFocus()
		return h, nil

	case TrafficTickMsg:
		if h.trafficMonitor != nil {
			sample, err := h.trafficMonitor.Sample()
			if err == nil {
				h.trafficChart.AddSample(sample)
			}
		}
		cmds = append(cmds, h.tickTrafficCmd())
		return h, tea.Batch(cmds...)

	case tea.WindowSizeMsg:
		h.Width = msg.Width
		h.Height = msg.Height
		cmds = append(cmds, h.footer.Update(tea.WindowSizeMsg{Width: h.Width, Height: 1}))

	case tea.BackgroundColorMsg:
		cmds = append(cmds, h.footer.Update(msg))
		cmds = append(cmds, h.console.Update(msg))
		cmds = append(cmds, h.trafficChart.Update(msg))
		cmds = append(cmds, h.processList.Update(msg))
		cmds = append(cmds, h.networkCard.Update(msg))
		cmds = append(cmds, h.editor.Update(msg))
		cmds = append(cmds, h.picker.Update(msg))
		return h, tea.Batch(cmds...)

	default:
		cmds = append(cmds, h.footer.Update(msg))
		cmds = append(cmds, h.console.Update(msg))
	}

	// Route keys to the currently focused component
	switch h.focusIndex {
	case 0:
		cmds = append(cmds, h.processList.Update(msg))
		h.applyFocus()
	case 1:
		cmds = append(cmds, h.networkCard.Update(msg))
	case 2:
		if _, ok := msg.(tea.KeyPressMsg); ok {
			cmds = append(cmds, h.console.Update(msg))
		}
	}

	return h, tea.Batch(cmds...)
}

func (h *Home) Render() string {
	h.trafficChart.TunnelActive = h.tunnelActive
	h.networkCard.ProxyModeLocked = h.tunnelActive || h.tunnelBusy || h.refreshingNetwork
	footer := h.footer
	if h.editor.Open {
		footer.Hints = []components.FooterHint{components.Hint("tab: switch field"), components.Hint("ctrl+s: save group"), components.Hint("esc: exit edit")}
	}
	if h.Width <= 0 || h.Height <= 0 {
		return "agywarp\n" + h.processList.Render() + "\n" + h.trafficChart.Render() + "\n" + h.console.Render() + "\n" + footer.Render()
	}

	if h.outputExpanded {
		height := h.Height - h.footer.Height
		if height < 1 {
			height = 1
		}
		h.console.SetSize(h.Width, height)
		footer.Hints = []components.FooterHint{components.Hint("o: dashboard"), components.Hint("PgUp/PgDn: scroll"), components.Hint("End: latest")}
		return lipgloss.JoinVertical(lipgloss.Left, h.console.Render(), footer.Render())
	}

	// 1. Calculate Heights
	footerHeight := h.footer.Height
	available := h.Height - footerHeight
	if available < 3 {
		available = 3
	}

	// Dynamic height distribution:
	// - Top cards: ~35%; traffic: ~20%; output: ~45%.
	trafficHeight := int(float64(available) * 0.20)
	if trafficHeight < 5 && available >= 14 {
		trafficHeight = 5
	}
	consoleHeight := int(float64(available) * 0.45)
	if consoleHeight < 4 && available >= 14 {
		consoleHeight = 4
	}
	cardsHeight := available - trafficHeight - consoleHeight
	if cardsHeight < 4 && available >= 14 {
		cardsHeight = 4
		trafficHeight = (available - cardsHeight) / 3
		consoleHeight = available - cardsHeight - trafficHeight
	}

	// Keep the outer-route section visible when the terminal has enough space.
	if available >= 22 && cardsHeight < 12 {
		cardsHeight = 12
		trafficHeight = (available - cardsHeight) / 3
		consoleHeight = available - cardsHeight - trafficHeight
	}

	// Give traffic four more rows while keeping at least four rows for output.
	extraTrafficRows := min(4, max(0, consoleHeight-4))
	trafficHeight += extraTrafficRows
	consoleHeight -= extraTrafficRows

	h.trafficChart.Width = h.Width
	h.trafficChart.Height = trafficHeight
	h.console.SetSize(h.Width, consoleHeight)

	// 2. Render Top Area
	var topContent string
	if h.editor.Open {
		// Group Editor modal overlays the top cards area
		h.editor.Width = h.Width
		h.editor.Height = cardsHeight
		topContent = h.editor.Render()
	} else if h.picker.Open {
		// Picker modal overlays the top cards area
		h.picker.Width = h.Width
		h.picker.Height = cardsHeight
		topContent = h.picker.Render()
	} else if h.Width >= 70 {
		leftWidth := int(float64(h.Width) * 0.56)
		rightWidth := h.Width - leftWidth

		h.processList.Width = leftWidth
		h.processList.Height = cardsHeight

		h.networkCard.Width = rightWidth
		h.networkCard.Height = cardsHeight

		topContent = lipgloss.JoinHorizontal(lipgloss.Top, h.processList.Render(), h.networkCard.Render())
	} else {
		// Narrow terminal fallback
		h.processList.Width = h.Width
		h.processList.Height = cardsHeight
		topContent = h.processList.Render()
	}

	// 3. Assemble full screen layout: Top (Cards) -> Middle (Traffic Chart) -> Bottom (Console) -> Footer
	renderedTop := lipgloss.NewStyle().
		Width(h.Width).
		Height(cardsHeight).
		MaxHeight(cardsHeight).
		Render(topContent)

	renderedTraffic := lipgloss.NewStyle().
		Width(h.Width).
		Height(trafficHeight).
		MaxHeight(trafficHeight).
		Render(h.trafficChart.Render())

	renderedConsole := lipgloss.NewStyle().
		Width(h.Width).
		Height(consoleHeight).
		MaxHeight(consoleHeight).
		Render(h.console.Render())

	renderedFooter := lipgloss.NewStyle().
		Width(h.Width).
		Height(h.footer.Height).
		MaxHeight(h.footer.Height).
		Render(footer.Render())

	screen := lipgloss.JoinVertical(
		lipgloss.Left,
		renderedTop,
		renderedTraffic,
		renderedConsole,
		renderedFooter,
	)

	// Invariant: The full screen layout NEVER exceeds h.Height, and the footer is ALWAYS anchored at the bottom
	screenLines := strings.Split(screen, "\n")
	footerLines := strings.Split(renderedFooter, "\n")

	if len(screenLines) > h.Height {
		topCount := h.Height - len(footerLines)
		if topCount < 0 {
			topCount = 0
		}
		var finalLines []string
		finalLines = append(finalLines, screenLines[:topCount]...)
		finalLines = append(finalLines, footerLines...)
		return strings.Join(finalLines, "\n")
	} else if len(screenLines) < h.Height {
		bodyCount := len(screenLines) - len(footerLines)
		if bodyCount < 0 {
			bodyCount = 0
		}
		bodyLines := screenLines[:bodyCount]
		for len(bodyLines)+len(footerLines) < h.Height {
			bodyLines = append(bodyLines, "")
		}
		var finalLines []string
		finalLines = append(finalLines, bodyLines...)
		finalLines = append(finalLines, footerLines...)
		return strings.Join(finalLines, "\n")
	}

	return screen
}
