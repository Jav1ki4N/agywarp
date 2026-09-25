package components

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// LogMsg represents a log event sent to the console
type LogMsg struct {
	Level   string // INFO, OK/SUCCESS, WARN, ERROR
	Message string
}

// LogEntry stores structured log data to support dynamic color re-rendering
type LogEntry struct {
	Time    time.Time
	Level   string
	Message string
}

// Console represents a terminal-like command output / log panel
type Console struct {
	ComponentBase
	Title       string
	Logs        []LogEntry
	maxLogs     int
	Active      bool
	SpinnerView string

	// Dynamic colors calculated from terminal background
	DelimFg color.Color
	TitleFg color.Color
	TimeFg  color.Color
	TextFg  color.Color
}

// NewConsole initializes a new Console component with default fallback colors
func NewConsole() Console {
	c := Console{
		ComponentBase: ComponentBase{Height: 8},
		Title:         "Output",
		maxLogs:       200,
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		TimeFg:        styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
	c.AddLog("INFO", "agywarp TUI initialized")
	c.AddLog("INFO", "ready for proxy orchestration")
	return c
}

// SetSize updates the width and height of the console component
func (c *Console) SetSize(w, h int) {
	c.Width = w
	c.Height = h
}

// AddLog appends a structured log entry to the buffer
func (c *Console) AddLog(level, message string) {
	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: message,
	}
	c.Logs = append(c.Logs, entry)
	if len(c.Logs) > c.maxLogs {
		c.Logs = c.Logs[len(c.Logs)-c.maxLogs:]
	}
}

// Clear clears all logs in the console buffer
func (c *Console) Clear() {
	c.Logs = nil
}

func (c *Console) Init() tea.Cmd {
	return nil
}

func (c *Console) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		// Dynamically elevate gray tones relative to actual terminal background
		c.DelimFg = styles.ElevateColor(msg, 35) // Subtle delimiter line
		c.TitleFg = styles.ElevateColor(msg, 75) // Output title
		c.TimeFg = styles.ElevateColor(msg, 65)  // Timestamp prefix
		c.TextFg = styles.ElevateColor(msg, 130) // Crisp, readable log text
	case LogMsg:
		c.AddLog(msg.Level, msg.Message)
	}
	return nil
}

func truncateVisualString(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		candidate := string(runes) + "…"
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return ""
}

func (c *Console) Render() string {
	if c.Width <= 0 || c.Height <= 0 {
		return ""
	}

	// Resolve dynamic colors with fallbacks
	delimFg := c.DelimFg
	if delimFg == nil {
		delimFg = styles.ColorDarkGray
	}
	titleFg := c.TitleFg
	if titleFg == nil {
		titleFg = styles.ColorDimGray
	}
	timeFg := c.TimeFg
	if timeFg == nil {
		timeFg = styles.ColorDimGray
	}
	textFg := c.TextFg
	if textFg == nil {
		textFg = styles.ColorLightGray
	}

	// 1. Light Delimiter with Title dividing Cards and Console
	delimStyle := lipgloss.NewStyle().Foreground(delimFg)
	titleStyle := lipgloss.NewStyle().Foreground(titleFg).Bold(false)

	titlePart := "── " + c.Title + " "
	titleWidth := lipgloss.Width(titlePart)

	var delimLine string
	if c.Width > titleWidth {
		filler := strings.Repeat("─", c.Width-titleWidth)
		delimLine = titleStyle.Render(titlePart) + delimStyle.Render(filler)
	} else {
		delimLine = delimStyle.Render(strings.Repeat("─", c.Width))
	}

	// If there's only 1 line of vertical space, just render the delimiter
	availableHeight := c.Height - 1
	if availableHeight <= 0 {
		return delimLine
	}

	// 2. Visible logs (Tail auto-scroll: display latest N entries)
	visibleLogs := c.Logs
	if len(visibleLogs) > availableHeight {
		visibleLogs = visibleLogs[len(visibleLogs)-availableHeight:]
	}

	renderedLines := make([]string, 0, availableHeight)
	for i, entry := range visibleLogs {
		ts := entry.Time.Format("15:04:05")
		var badge string
		var badgeWidth int

		switch strings.ToUpper(entry.Level) {
		case "OK", "SUCCESS":
			badge = lipgloss.NewStyle().Foreground(styles.ColorSuccess).Render("[OK]")
			badgeWidth = 4
		case "WARN", "WARNING":
			badge = lipgloss.NewStyle().Foreground(styles.ColorWarning).Render("[WARN]")
			badgeWidth = 6
		case "ERR", "ERROR":
			badge = lipgloss.NewStyle().Foreground(styles.ColorDanger).Render("[ERR]")
			badgeWidth = 5
		default:
			badge = lipgloss.NewStyle().Foreground(titleFg).Render("[INFO]")
			badgeWidth = 6
		}

		// Clean message: remove newlines & carriage returns to strictly maintain single line
		cleanMsg := strings.ReplaceAll(entry.Message, "\r", "")
		cleanMsg = strings.ReplaceAll(cleanMsg, "\n", " ")

		// If this is the latest in-progress entry while active, append spinner
		var spinnerSuffix string
		var spinnerWidth int
		if c.Active && c.SpinnerView != "" && i == len(visibleLogs)-1 && strings.HasSuffix(cleanMsg, "...") {
			spinnerSuffix = " " + c.SpinnerView
			spinnerWidth = 1 + lipgloss.Width(c.SpinnerView)
		}

		// Truncate message text so it fits within c.Width without wrapping
		// prefix: ts (8) + space (1) + badge + space (1)
		prefixWidth := 8 + 1 + badgeWidth + 1 + spinnerWidth
		maxMsgWidth := c.Width - prefixWidth
		if maxMsgWidth > 0 && lipgloss.Width(cleanMsg) > maxMsgWidth {
			cleanMsg = truncateVisualString(cleanMsg, maxMsgWidth)
		}

		tsStyled := lipgloss.NewStyle().Foreground(timeFg).Render(ts)
		msgStyled := lipgloss.NewStyle().Foreground(textFg).Render(cleanMsg)
		line := fmt.Sprintf("%s %s %s%s", tsStyled, badge, msgStyled, spinnerSuffix)
		renderedLines = append(renderedLines, line)
	}

	// Pad with blank lines if fewer entries exist to guarantee constant console height
	for len(renderedLines) < availableHeight {
		renderedLines = append(renderedLines, "")
	}

	// Clamping: ensure exactly availableHeight lines
	if len(renderedLines) > availableHeight {
		renderedLines = renderedLines[len(renderedLines)-availableHeight:]
	}

	logContent := strings.Join(renderedLines, "\n")
	boxStyle := lipgloss.NewStyle().
		Width(c.Width).
		Height(availableHeight).
		MaxHeight(availableHeight)

	renderedLogs := boxStyle.Render(logContent)

	return lipgloss.JoinVertical(lipgloss.Left, delimLine, renderedLogs)
}
