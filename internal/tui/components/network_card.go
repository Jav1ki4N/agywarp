package components

import (
	"fmt"
	"image/color"
	"strings"

	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type NetworkCard struct {
	ComponentBase
	Title           string
	ServiceStatus   string
	WarpStatus      string
	Protocol        string
	ProxyMode       string
	ProxyModeLocked bool
	ExitCountry     string
	ExitIP          string
	Colo            string
	Latency         string
	MihomoRule      string
	OuterNode       string
	OuterProvider   string
	OuterAirport    string
	OuterStatus     string

	Refreshing  bool
	SpinnerView string

	// Dynamic grays calculated from terminal background
	DelimFg  color.Color
	TitleFg  color.Color
	LabelFg  color.Color
	DimValFg color.Color
	TextFg   color.Color
}

func NewNetworkCard() NetworkCard {
	return NetworkCard{
		ComponentBase: ComponentBase{Height: 8},
		Title:         "Network & Tunnel",
		ServiceStatus: "INACTIVE [OFF]",
		WarpStatus:    "DISCONNECTED",
		Protocol:      "---",
		ProxyMode:     "SOCKS5",
		ExitCountry:   "---",
		ExitIP:        "---",
		Colo:          "---",
		Latency:       "---",
		MihomoRule:    "UNCONFIGURED",
		OuterNode:     "---",
		OuterProvider: "---",
		OuterAirport:  "---",
		OuterStatus:   "UNOBSERVED",
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		LabelFg:       styles.ColorDimGray,
		DimValFg:      styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
}

// ToggleTunnelMsg is emitted when the user presses 'space' while NetworkCard is focused.
type ToggleTunnelMsg struct{}

// RefreshNetworkMsg is emitted when the user presses 'r' to refresh network and tunnel status.
type RefreshNetworkMsg struct{}

func (n *NetworkCard) Init() tea.Cmd {
	return nil
}

func (n *NetworkCard) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case " ", "space":
			return func() tea.Msg {
				return ToggleTunnelMsg{}
			}
		case "r", "R":
			return func() tea.Msg {
				return RefreshNetworkMsg{}
			}
		}
	case tea.BackgroundColorMsg:
		n.DelimFg = styles.ElevateColor(msg, 35)
		n.TitleFg = styles.ElevateColor(msg, 75)
		n.LabelFg = styles.ElevateColor(msg, 65)
		n.DimValFg = styles.ElevateColor(msg, 55) // Dimmer gray for default / unlaunched values
		n.TextFg = styles.ElevateColor(msg, 130)
	}
	return nil
}

func (n *NetworkCard) Render() string {
	if n.Width <= 0 || n.Height <= 0 {
		return ""
	}

	// Resolve dynamic colors with fallbacks
	delimFg := n.DelimFg
	if delimFg == nil {
		delimFg = styles.ColorDarkGray
	}
	titleFg := n.TitleFg
	if titleFg == nil {
		titleFg = styles.ColorDimGray
	}
	labelFg := n.LabelFg
	if labelFg == nil {
		labelFg = styles.ColorDimGray
	}
	dimValFg := n.DimValFg
	if dimValFg == nil {
		dimValFg = styles.ColorDimGray
	}
	textFg := n.TextFg
	if textFg == nil {
		textFg = styles.ColorLightGray
	}

	// 1. Title formatting (Highlight title when Focused)
	var titleStyle lipgloss.Style
	var delimStyle lipgloss.Style

	if n.Focused {
		titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.ColorPrimary)
		delimStyle = lipgloss.NewStyle().Foreground(styles.ColorDimGray)
	} else {
		titleStyle = lipgloss.NewStyle().
			Bold(false).
			Foreground(titleFg)
		delimStyle = lipgloss.NewStyle().Foreground(delimFg)
	}

	titleText := "── " + n.Title + " "
	titleWidth := lipgloss.Width(titleText)

	var headerLine string
	if n.Width > titleWidth {
		headerLine = titleStyle.Render(titleText) + delimStyle.Render(strings.Repeat("─", n.Width-titleWidth))
	} else {
		headerLine = delimStyle.Render(strings.Repeat("─", n.Width))
	}

	availableHeight := n.Height - 1
	if availableHeight <= 0 {
		return headerLine
	}

	// 2. Render Metric Rows (Dimmer colors for unlaunched / default values)
	renderRow := func(label, value string, isDefault bool, activeColor color.Color) string {
		lblStr := lipgloss.NewStyle().Foreground(labelFg).Render(fmt.Sprintf("%-11s", label))

		var valStr string
		if isDefault {
			// Dimmer tone for unlaunched state
			valStr = lipgloss.NewStyle().Foreground(dimValFg).Render(value)
		} else {
			valStr = lipgloss.NewStyle().Foreground(activeColor).Bold(true).Render(value)
		}
		return ansi.Truncate(fmt.Sprintf("  %s %s", lblStr, valStr), n.Width, "…")
	}

	// Determine whether each field is at its default unlaunched value
	warpDefault := (n.WarpStatus == "DISCONNECTED" || n.WarpStatus == "---" || n.WarpStatus == "DOWN")
	exitRouteDefault := (n.ExitCountry == "---" || n.ExitCountry == "UNVERIFIED")
	exitIPDefault := (n.ExitIP == "---")
	latencyDefault := (n.Latency == "---")
	mihomoDefault := (n.MihomoRule == "UNCONFIGURED" || n.MihomoRule == "INACTIVE" || n.MihomoRule == "---")

	warpVal := n.WarpStatus
	if n.Refreshing && warpDefault && n.SpinnerView != "" {
		warpVal = n.SpinnerView + " checking..."
	} else if n.Protocol != "" && n.Protocol != "---" {
		warpVal = fmt.Sprintf("%s (%s)", n.WarpStatus, n.Protocol)
	}

	exitRouteVal := n.ExitCountry
	if n.Refreshing && exitRouteDefault && n.SpinnerView != "" {
		exitRouteVal = n.SpinnerView + " checking..."
	}

	exitIPVal := n.ExitIP
	if n.Refreshing && exitIPDefault && n.SpinnerView != "" {
		exitIPVal = n.SpinnerView + " resolving..."
	} else if n.Colo != "" && n.Colo != "---" {
		exitIPVal = fmt.Sprintf("%s · %s", n.ExitIP, n.Colo)
	}

	latencyVal := n.Latency
	if n.Refreshing && n.SpinnerView != "" {
		if latencyDefault {
			latencyVal = n.SpinnerView + " measuring..."
		} else {
			latencyVal = fmt.Sprintf("%s (%s)", n.Latency, n.SpinnerView)
		}
	}

	mihomoVal := n.MihomoRule
	if n.Refreshing && mihomoDefault && n.SpinnerView != "" {
		mihomoVal = n.SpinnerView + " inspecting..."
	}

	serviceDefault := (n.ServiceStatus == "INACTIVE [OFF]" || n.ServiceStatus == "INACTIVE" || n.ServiceStatus == "")
	serviceColor := styles.ColorSuccess
	if serviceDefault {
		serviceColor = dimValFg
	}
	serviceVal := n.ServiceStatus
	if serviceVal == "" {
		serviceVal = "INACTIVE [OFF]"
	}

	pathColor := styles.ColorWarning
	if strings.HasPrefix(n.OuterStatus, "OBSERVED") {
		pathColor = styles.ColorSuccess
	} else if n.OuterStatus == "MISMATCH" {
		pathColor = styles.ColorDanger
	}
	modeVal := n.ProxyMode + " [p switch]"
	if n.ProxyModeLocked {
		modeVal = n.ProxyMode + " [locked]"
	}
	lines := []string{
		renderRow("Service", serviceVal, serviceDefault, serviceColor),
		renderRow("Proxy Mode", modeVal, false, textFg),
		renderRow("WARP Tunnel", warpVal, warpDefault, styles.ColorSuccess),
		renderRow("Exit Route", exitRouteVal, exitRouteDefault, styles.ColorWhite),
		renderRow("Exit IP", exitIPVal, exitIPDefault, textFg),
		renderRow("Latency", latencyVal, latencyDefault, styles.ColorWarning),
		renderRow("Mihomo Rule", mihomoVal, mihomoDefault, styles.ColorSuccess),
		delimStyle.Render("  ── WARP outer route"),
		renderRow("Node", n.OuterNode, n.OuterNode == "---", textFg),
		renderRow("Provider", n.OuterProvider, n.OuterProvider == "---", textFg),
		renderRow("Airport", n.OuterAirport, n.OuterAirport == "---", textFg),
		renderRow("Path", n.OuterStatus, false, pathColor),
	}

	if len(lines) > availableHeight {
		lines = lines[:availableHeight]
	}

	content := strings.Join(lines, "\n")
	boxStyle := lipgloss.NewStyle().
		Width(n.Width).
		Height(availableHeight)

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, boxStyle.Render(content))
}
