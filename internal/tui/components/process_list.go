package components

import (
	"fmt"
	"image/color"
	"strings"

	"agywarp/internal/process"
	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type ProcessToggledMsg struct {
	Profile process.Profile
}

type ProcessDeletedMsg struct {
	Profile process.Profile
}

type OpenProcessPickerMsg struct{}
type OpenGroupCreateMsg struct{}
type OpenGroupEditMsg struct {
	Profile process.Profile
}

type ProcessList struct {
	ComponentBase
	Title         string
	Profiles      []process.Profile
	RunningCounts map[string]int
	Cursor        int
	Offset        int // Viewport scroll offset

	// Dynamic grays calculated from terminal background
	DelimFg color.Color
	TitleFg color.Color
	DimFg   color.Color
	TextFg  color.Color
}

func NewProcessList() ProcessList {
	return ProcessList{
		ComponentBase: ComponentBase{Height: 8},
		Title:         "Process Profiles",
		Profiles:      nil, // Default: no profiles configured yet
		RunningCounts: make(map[string]int),
		Cursor:        0,
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		DimFg:         styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
}

func (p *ProcessList) Init() tea.Cmd {
	return nil
}

func (p *ProcessList) Selected() *process.Profile {
	if len(p.Profiles) == 0 || p.Cursor < 0 || p.Cursor >= len(p.Profiles) {
		return nil
	}
	return &p.Profiles[p.Cursor]
}

func (p *ProcessList) ToggleSelected() (process.Profile, bool) {
	if len(p.Profiles) == 0 || p.Cursor < 0 || p.Cursor >= len(p.Profiles) {
		return process.Profile{}, false
	}
	p.Profiles[p.Cursor].Enabled = !p.Profiles[p.Cursor].Enabled
	return p.Profiles[p.Cursor], true
}

func (p *ProcessList) DeleteSelected() (process.Profile, bool) {
	if len(p.Profiles) == 0 || p.Cursor < 0 || p.Cursor >= len(p.Profiles) {
		return process.Profile{}, false
	}
	deleted := p.Profiles[p.Cursor]
	p.Profiles = append(p.Profiles[:p.Cursor], p.Profiles[p.Cursor+1:]...)
	if p.Cursor >= len(p.Profiles) && p.Cursor > 0 {
		p.Cursor = len(p.Profiles) - 1
	}
	return deleted, true
}

func (p *ProcessList) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		p.DelimFg = styles.ElevateColor(msg, 35)
		p.TitleFg = styles.ElevateColor(msg, 75)
		p.DimFg = styles.ElevateColor(msg, 55) // Dimmer gray for default / inactive values
		p.TextFg = styles.ElevateColor(msg, 130)
		return nil
	}

	if !p.Focused {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "up", "k":
			if p.Cursor > 0 {
				p.Cursor--
			}
		case "down", "j":
			if p.Cursor < len(p.Profiles)-1 {
				p.Cursor++
			}
		case "a":
			return func() tea.Msg {
				return OpenGroupCreateMsg{}
			}
		case "e":
			if sel := p.Selected(); sel != nil {
				return func() tea.Msg {
					return OpenGroupEditMsg{Profile: *sel}
				}
			}
		case " ", "space":
			if prof, ok := p.ToggleSelected(); ok {
				return func() tea.Msg {
					return ProcessToggledMsg{Profile: prof}
				}
			}
		case "d":
			if prof, ok := p.DeleteSelected(); ok {
				return func() tea.Msg {
					return ProcessDeletedMsg{Profile: prof}
				}
			}
		}
	}
	return nil
}

func (p *ProcessList) Render() string {
	if p.Width <= 0 || p.Height <= 0 {
		return ""
	}

	// Resolve dynamic colors with fallbacks
	delimFg := p.DelimFg
	if delimFg == nil {
		delimFg = styles.ColorDarkGray
	}
	titleFg := p.TitleFg
	if titleFg == nil {
		titleFg = styles.ColorDimGray
	}
	dimFg := p.DimFg
	if dimFg == nil {
		dimFg = styles.ColorDimGray
	}
	textFg := p.TextFg
	if textFg == nil {
		textFg = styles.ColorLightGray
	}

	// 1. Title formatting (Highlight title when Focused)
	var titleStyle lipgloss.Style
	var delimStyle lipgloss.Style

	if p.Focused {
		titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.ColorPrimary)
		delimStyle = lipgloss.NewStyle().Foreground(delimFg)
	} else {
		titleStyle = lipgloss.NewStyle().
			Bold(false).
			Foreground(titleFg)
		delimStyle = lipgloss.NewStyle().Foreground(delimFg)
	}

	titleText := "── " + p.Title + " "
	titleWidth := lipgloss.Width(titleText)

	var headerLine string
	if p.Width > titleWidth {
		headerLine = titleStyle.Render(titleText) + delimStyle.Render(strings.Repeat("─", p.Width-titleWidth))
	} else {
		headerLine = delimStyle.Render(strings.Repeat("─", p.Width))
	}

	availableHeight := p.Height - 1
	if availableHeight <= 0 {
		return headerLine
	}

	// 2. Render Profile Rows or Default Empty State
	var lines []string

	if len(p.Profiles) == 0 {
		emptyMsg := lipgloss.NewStyle().
			Foreground(dimFg).
			Render("  (no profile configured)")
		lines = append(lines, emptyMsg)
	} else {
		// Auto-roll / Windowed scrolling: keep Cursor in visible view
		if p.Cursor < p.Offset {
			p.Offset = p.Cursor
		} else if p.Cursor >= p.Offset+availableHeight {
			p.Offset = p.Cursor - availableHeight + 1
		}
		maxOffset := max(0, len(p.Profiles)-availableHeight)
		if p.Offset > maxOffset {
			p.Offset = maxOffset
		}
		if p.Offset < 0 {
			p.Offset = 0
		}

		endIdx := min(len(p.Profiles), p.Offset+availableHeight)
		for i := p.Offset; i < endIdx; i++ {
			prof := p.Profiles[i]
			isSelected := p.Focused && (i == p.Cursor)

			// Cursor indicator
			cursorStr := "  "
			if isSelected {
				cursorStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary).Render("> ")
			}

			// Enabled / Disabled dot
			dotStr := "○"
			if prof.Enabled {
				dotStr = lipgloss.NewStyle().Foreground(styles.ColorSuccess).Render("●")
			} else {
				dotStr = lipgloss.NewStyle().Foreground(dimFg).Render("○")
			}

			// Profile label
			labelStyle := lipgloss.NewStyle()
			if isSelected {
				labelStyle = labelStyle.Bold(true).Foreground(styles.ColorWhite)
			} else if prof.Enabled {
				labelStyle = labelStyle.Foreground(textFg)
			} else {
				labelStyle = labelStyle.Foreground(dimFg)
			}
			labelStr := labelStyle.Render(prof.Label)

			// Running instances & paths indicator
			instances := p.RunningCounts[prof.ID]
			pathInfo := ""
			if len(prof.Matchers) > 1 {
				pathInfo = fmt.Sprintf(" (%d paths)", len(prof.Matchers))
			}

			var runStr string
			if instances > 0 {
				runStr = lipgloss.NewStyle().Foreground(styles.ColorSuccess).
					Render(fmt.Sprintf("RUNNING · %d inst%s", instances, pathInfo))
			} else {
				runStr = lipgloss.NewStyle().Foreground(dimFg).
					Render(fmt.Sprintf("NOT RUNNING%s", pathInfo))
			}

			// Toggle badge [ON] / OFF
			var badgeStr string
			if prof.Enabled {
				badgeStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorSuccess).Render("[ON]")
			} else {
				badgeStr = lipgloss.NewStyle().Foreground(dimFg).Render("OFF")
			}

			// Left column (Cursor + Dot + Label)
			leftPart := fmt.Sprintf("%s%s %s", cursorStr, dotStr, labelStr)
			leftWidth := lipgloss.Width(leftPart)

			// Middle (Running status) and Right (Badge)
			rightPart := fmt.Sprintf("%s    %s", runStr, badgeStr)
			rightWidth := lipgloss.Width(rightPart)

			gap := p.Width - leftWidth - rightWidth - 1
			if gap < 2 {
				gap = 2
			}

			row := fmt.Sprintf("%s%s%s", leftPart, strings.Repeat(" ", gap), rightPart)
			lines = append(lines, row)
		}
	}

	// 3. Assemble content box
	content := strings.Join(lines, "\n")
	boxStyle := lipgloss.NewStyle().
		Width(p.Width).
		Height(availableHeight)

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, boxStyle.Render(content))
}
