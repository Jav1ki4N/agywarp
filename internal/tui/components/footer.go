package components

import (
	"image/color"
	"strings"

	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		BarBg:         styles.ColorBarBg,
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
		f.BarBg = styles.ElevateColor(msg, 22)
		f.HintFg = styles.ElevateColor(msg, 95)
	}
	return nil
}

func (f *Footer) Render() string {
	if f.Width <= 0 { // nothing can be shown
		return ""
	}

	// version block
	versionStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWhite).
		Background(styles.ColorPrimary).
		Padding(0, 1)
	versionBlock := versionStyle.Render(f.Version)

	// status(literal) & state
	statusLabelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWhite).
		Background(styles.ColorStatusDarkBg).
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

	// 使用动态计算出的微亮背景色（若未取到则回退到预设值）
	barBg := f.BarBg
	hintFg := f.HintFg
	if barBg == nil || hintFg == nil {
		barBg = styles.ColorBarBg
		hintFg = styles.ColorHintFg
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
			renderedHints = append(renderedHints, lipgloss.NewStyle().
				Foreground(fg).
				Bold(bold).
				Render(h.Text))
		}
		hintsStr := strings.Join(renderedHints, "    ")
		hintContainer := lipgloss.NewStyle().
			Background(barBg).
			Padding(0, 1)

		hintsRendered := hintContainer.Render(hintsStr)
		hintsWidth := lipgloss.Width(hintsRendered)

		if hintsWidth <= middleWidth {
			fillerWidth := middleWidth - hintsWidth
			filler := lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", fillerWidth))
			middleSection = lipgloss.JoinHorizontal(lipgloss.Top, hintsRendered, filler)
		} else {
			// 空间不足以完整显示提示时，退化为纯背景底条
			middleSection = lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", middleWidth))
		}
	}

	// 单行状态栏：Version | Hints (自适应动态微亮区) | Status + State
	return lipgloss.JoinHorizontal(lipgloss.Top, versionBlock, middleSection, statusStateGroup)
}
