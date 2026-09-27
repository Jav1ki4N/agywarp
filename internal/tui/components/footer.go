package components

import (
	"image/color"
	"strings"

	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type FooterHint struct {
	Text  string
	Color color.Color
}

// Hint creates a FooterHint with optional color styling.
func Hint(text string, c ...color.Color) FooterHint {
	var clr color.Color
	if len(c) > 0 {
		clr = c[0]
	}
	return FooterHint{Text: text, Color: clr}
}

type Footer struct {
	ComponentBase
	Hints       []FooterHint
	Version     string
	Status      string
	State       string
	StateBg     color.Color
	Refreshing  bool
	SpinnerView string
	BarBg       color.Color
	HintFg      color.Color
}

func NewFooter() Footer {
	return Footer{
		ComponentBase: ComponentBase{Height: 1},
		Hints:         []FooterHint{Hint("q: quit")},
		Version:       "v0.1.0-dev",
		Status:        "STATUS",
		State:         "DOWN",
		StateBg:       styles.ColorDanger,
		HintFg:        styles.ColorHintFg,
	}
}

func (f *Footer) Init() tea.Cmd {
	return nil
}

func (f *Footer) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg: // get terminal window size
		f.Width = msg.Width
		f.Height = 1
	case tea.BackgroundColorMsg: // get terminal background color
		r, g, b, _ := msg.RGBA()
		f.BarBg = color.RGBA{R: uint8(float64(r>>8) * 0.85), G: uint8(float64(g>>8) * 0.85), B: uint8(float64(b>>8) * 0.85), A: 255}
		f.HintFg = styles.ElevateColor(msg, 95)
	}
	return nil
}

func (f *Footer) Render() string {
	if f.Width <= 0 { // nothing can be shown
		return ""
	}

	// Until the terminal reports its background, leave the bar transparent.
	barBg := f.BarBg
	hintFg := f.HintFg
	if hintFg == nil {
		hintFg = styles.ColorHintFg
	}
	barStyle := lipgloss.NewStyle()
	if barBg != nil {
		barStyle = barStyle.Background(barBg)
	}

	// version block
	versionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWhite).
		Background(styles.ColorPrimary).
		Padding(0, 1)
	versionBlock := versionStyle.Render(f.Version)

	// status(literal) & state
	statusLabelStyle := barStyle.
		Bold(true).
		Foreground(hintFg).
		Padding(0, 1)
	statusLabelBlock := statusLabelStyle.Render(f.Status)

	stateBg := f.StateBg
	if stateBg == nil {
		if f.State == "UP" {
			stateBg = styles.ColorSuccess
		} else {
			stateBg = styles.ColorDanger
		}
	}

	stateStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWhite).
		Background(stateBg).
		Padding(0, 1)
	stateBlock := stateStyle.Render(f.State)

	statusStateGroup := lipgloss.JoinHorizontal(lipgloss.Top, statusLabelBlock, stateBlock)

	// hints
	leftWidth := lipgloss.Width(versionBlock)
	rightWidth := lipgloss.Width(statusStateGroup)
	middleWidth := f.Width - leftWidth - rightWidth
	if middleWidth < 0 {
		middleWidth = 0
	}

	var middleSection string
	if middleWidth > 0 {
		var renderedHints []string
		for _, h := range f.Hints {
			fg := hintFg
			bold := false
			if h.Color != nil {
				fg = h.Color
				bold = true
			}
			renderedHints = append(renderedHints, lipgloss.NewStyle().Foreground(fg).Bold(bold).Render(strings.ReplaceAll(strings.ReplaceAll(h.Text, "\n", " "), "\r", "")))
		}
		separator := "    "
		hintsStr := strings.Join(renderedHints, separator)
		if middleWidth > 2 {
			hintsStr = ansi.Truncate(hintsStr, middleWidth-2, "…")
			padding := middleWidth - lipgloss.Width(hintsStr)
			middleSection = strings.Repeat(" ", padding/2) + hintsStr + strings.Repeat(" ", padding-padding/2)
		} else {
			middleSection = strings.Repeat(" ", middleWidth)
		}
	}

	// 单行状态栏：Version | Hints (仅前景色) | Status + State
	return ansi.Truncate(versionBlock+middleSection+statusStateGroup, f.Width, "")
}
