package components

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strings"

	"agywarp/internal/clipboard"
	"agywarp/internal/process"
	"agywarp/internal/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type ProfileAddedMsg struct {
	Profile process.Profile
}

// ExecutableItem represents a distinct binary belonging to a process group
type ExecutableItem struct {
	Executable string
	Command    string
	Instances  int
	Selected   bool // Whether user checked it to stay in this group
}

// ProcessGroupCandidate groups processes sharing the same comm/name
type ProcessGroupCandidate struct {
	Name        string
	Executables []*ExecutableItem
	TotalInst   int
	Expanded    bool
}

// PickerRow represents a flattened navigable line in the group tree
type PickerRow struct {
	IsGroupHeader bool
	GroupIndex    int
	ExecIndex     int
}

type ProcessPicker struct {
	ComponentBase
	Open        bool
	Mode        int // 0: Select from Running Groups, 1: Enter Custom Path
	SearchQuery string
	CustomInput string
	Cursor      int
	Offset      int // Viewport scroll offset

	AllRunning     []process.RunningProcess
	AllGroups      []*ProcessGroupCandidate
	FilteredGroups []*ProcessGroupCandidate
	Rows           []PickerRow

	Scanning    bool
	SpinnerView string

	// Dynamic grays
	DelimFg color.Color
	TitleFg color.Color
	DimFg   color.Color
	TextFg  color.Color
}

func NewProcessPicker() ProcessPicker {
	return ProcessPicker{
		ComponentBase: ComponentBase{Height: 10},
		Open:          false,
		Mode:          0,
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		DimFg:         styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
}

func (p *ProcessPicker) SetRunningProcesses(procs []process.RunningProcess) {
	p.AllRunning = procs
	p.rebuildGroups()
	p.filterCandidates()
}

func (p *ProcessPicker) rebuildGroups() {
	groupMap := make(map[string]*ProcessGroupCandidate)
	var order []string

	for _, proc := range p.AllRunning {
		name := proc.Name
		if name == "" {
			if proc.Executable != "" {
				name = filepath.Base(proc.Executable)
			} else {
				continue
			}
		}

		exe := proc.Executable
		if exe == "" {
			exe = name
		}

		cmdPreview := strings.Join(proc.Command, " ")

		group, exists := groupMap[name]
		if !exists {
			group = &ProcessGroupCandidate{
				Name:     name,
				Expanded: true, // Default expanded so user sees paths
			}
			groupMap[name] = group
			order = append(order, name)
		}

		group.TotalInst++

		// Find or append executable item under group
		found := false
		for _, item := range group.Executables {
			if item.Executable == exe {
				item.Instances++
				found = true
				break
			}
		}
		if !found {
			group.Executables = append(group.Executables, &ExecutableItem{
				Executable: exe,
				Command:    cmdPreview,
				Instances:  1,
				Selected:   true, // Checked by default
			})
		}
	}

	p.AllGroups = make([]*ProcessGroupCandidate, 0, len(order))
	for _, k := range order {
		p.AllGroups = append(p.AllGroups, groupMap[k])
	}
}

func (p *ProcessPicker) filterCandidates() {
	query := strings.ToLower(strings.TrimSpace(p.SearchQuery))

	var filtered []*ProcessGroupCandidate
	for _, group := range p.AllGroups {
		if query == "" {
			filtered = append(filtered, group)
			continue
		}

		matchName := strings.Contains(strings.ToLower(group.Name), query)
		matchExe := false
		for _, item := range group.Executables {
			if strings.Contains(strings.ToLower(item.Executable), query) ||
				strings.Contains(strings.ToLower(item.Command), query) {
				matchExe = true
				break
			}
		}

		if matchName || matchExe {
			filtered = append(filtered, group)
		}
	}

	p.FilteredGroups = filtered
	p.rebuildRows()
}

func (p *ProcessPicker) rebuildRows() {
	var rows []PickerRow
	for gIdx, group := range p.FilteredGroups {
		rows = append(rows, PickerRow{IsGroupHeader: true, GroupIndex: gIdx})
		if group.Expanded {
			for eIdx := range group.Executables {
				rows = append(rows, PickerRow{IsGroupHeader: false, GroupIndex: gIdx, ExecIndex: eIdx})
			}
		}
	}
	p.Rows = rows

	if p.Cursor >= len(p.Rows) {
		p.Cursor = max(0, len(p.Rows)-1)
	}
	if p.Offset > p.Cursor {
		p.Offset = p.Cursor
	}
}

func (p *ProcessPicker) Init() tea.Cmd {
	return nil
}

func (p *ProcessPicker) handlePaste(content string) {
	content = strings.ReplaceAll(content, "\r", "")
	cleaned := strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	if cleaned == "" {
		return
	}
	if p.Mode == 0 {
		p.SearchQuery += cleaned
		p.filterCandidates()
	} else {
		p.CustomInput += cleaned
	}
}

func (p *ProcessPicker) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		p.DelimFg = styles.ElevateColor(msg, 35)
		p.TitleFg = styles.ElevateColor(msg, 75)
		p.DimFg = styles.ElevateColor(msg, 55)
		p.TextFg = styles.ElevateColor(msg, 130)
		return nil

	case tea.PasteMsg:
		if !p.Open {
			return nil
		}
		p.handlePaste(msg.Content)
		return nil
	}

	if !p.Open {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()

		switch key {
		case "esc":
			p.Open = false
			p.SearchQuery = ""
			p.CustomInput = ""
			return nil

		case "tab":
			// Toggle mode: 0 (Running groups) <-> 1 (Custom path)
			p.Mode = (p.Mode + 1) % 2
			return nil

		case "ctrl+v":
			if text := clipboard.ReadText(); text != "" {
				p.handlePaste(text)
			}
			return nil

		case " ", "space":
			// In Running Groups mode: toggle checkbox or collapse group
			if p.Mode == 0 && len(p.Rows) > 0 && p.Cursor < len(p.Rows) {
				row := p.Rows[p.Cursor]
				group := p.FilteredGroups[row.GroupIndex]

				if row.IsGroupHeader {
					// Toggle collapse / expand
					group.Expanded = !group.Expanded
					p.rebuildRows()
				} else {
					// Toggle inclusion in group
					item := group.Executables[row.ExecIndex]
					item.Selected = !item.Selected
				}
				return nil
			} else if p.Mode == 1 {
				p.CustomInput += " "
				return nil
			}

		case "enter":
			if p.Mode == 0 {
				if len(p.Rows) > 0 && p.Cursor < len(p.Rows) {
					row := p.Rows[p.Cursor]
					group := p.FilteredGroups[row.GroupIndex]

					// Gather selected executables in this group
					var matchers []process.Matcher
					for _, item := range group.Executables {
						if item.Selected {
							matchers = append(matchers, process.Matcher{
								Kind:    process.MatchExecutablePath,
								Pattern: item.Executable,
							})
						}
					}

					// If user unselected all, fallback to the current highlighted item
					if len(matchers) == 0 {
						if !row.IsGroupHeader && row.ExecIndex < len(group.Executables) {
							item := group.Executables[row.ExecIndex]
							matchers = append(matchers, process.Matcher{
								Kind:    process.MatchExecutablePath,
								Pattern: item.Executable,
							})
						} else if len(group.Executables) > 0 {
							matchers = append(matchers, process.Matcher{
								Kind:    process.MatchExecutablePath,
								Pattern: group.Executables[0].Executable,
							})
						}
					}

					prof := process.NewGroupProfile(group.Name, matchers, true)
					p.Open = false
					p.SearchQuery = ""
					return func() tea.Msg { return ProfileAddedMsg{Profile: prof} }
				}
			} else {
				raw := strings.TrimSpace(p.CustomInput)
				if raw != "" {
					res, err := process.ValidatePath(raw, p.AllRunning)
					if err != nil {
						rawErr := raw
						return func() tea.Msg {
							return LogMsg{
								Level:   "ERR",
								Message: fmt.Sprintf("Custom path does not exist in system: %s (%v)", rawErr, err),
							}
						}
					}
					label := filepath.Base(raw)
					kind := process.MatchExecutablePath
					if !strings.HasPrefix(raw, "/") {
						kind = process.MatchProcessName
					}
					pathToAdd := raw
					if strings.HasPrefix(raw, "~") && res.ResolvedPath != "" {
						pathToAdd = res.ResolvedPath
					}
					prof := process.NewProfile(label, kind, pathToAdd, true)
					p.Open = false
					p.CustomInput = ""
					return func() tea.Msg { return ProfileAddedMsg{Profile: prof} }
				}
			}

		case "up", "ctrl+p":
			if p.Mode == 0 && p.Cursor > 0 {
				p.Cursor--
			}
		case "down", "ctrl+n":
			if p.Mode == 0 && p.Cursor < len(p.Rows)-1 {
				p.Cursor++
			}

		case "backspace":
			if p.Mode == 0 {
				r := []rune(p.SearchQuery)
				if len(r) > 0 {
					p.SearchQuery = string(r[:len(r)-1])
					p.filterCandidates()
				}
			} else {
				r := []rune(p.CustomInput)
				if len(r) > 0 {
					p.CustomInput = string(r[:len(r)-1])
				}
			}

		default:
			// Append printable characters to active search or path field
			k := msg.Key()
			text := k.Text
			if text == "" && (k.Code == tea.KeySpace || key == "space") {
				text = " "
			}
			if text != "" {
				p.handlePaste(text)
			}
		}
	}

	return nil
}

func (p *ProcessPicker) Render() string {
	if !p.Open || p.Width <= 0 || p.Height <= 0 {
		return ""
	}

	dimFg := p.DimFg
	if dimFg == nil {
		dimFg = styles.ColorDimGray
	}
	textFg := p.TextFg
	if textFg == nil {
		textFg = styles.ColorLightGray
	}

	// 1. Header with Title & Mode Switcher
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary)
	delimStyle := lipgloss.NewStyle().Foreground(p.DelimFg)

	var modeRunningStr, modeCustomStr string
	if p.Mode == 0 {
		modeRunningStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorSuccess).Render("[● Running Groups]")
		modeCustomStr = lipgloss.NewStyle().Foreground(dimFg).Render("[○ Custom Path]")
	} else {
		modeRunningStr = lipgloss.NewStyle().Foreground(dimFg).Render("[○ Running Groups]")
		modeCustomStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorSuccess).Render("[● Custom Path]")
	}

	titleText := "── Add Process Group  " + modeRunningStr + " " + modeCustomStr + " "
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

	var lines []string

	// 2. Mode-specific content
	if p.Mode == 0 {
		// Running Process Groups: Search box + Tree rows
		searchPrompt := lipgloss.NewStyle().Foreground(dimFg).Render("Search: ")
		searchVal := lipgloss.NewStyle().Foreground(textFg).Bold(true).Render(p.SearchQuery + "_")
		lines = append(lines, fmt.Sprintf("  %s%s", searchPrompt, searchVal))

		maxListLines := availableHeight - 2 // reserve 1 for search, 1 for bottom hints
		if maxListLines <= 0 {
			maxListLines = 1
		}

		if len(p.Rows) == 0 {
			emptyMsg := lipgloss.NewStyle().Foreground(dimFg).Render("  (no matching running processes)")
			lines = append(lines, emptyMsg)
		} else {
			// Viewport auto-roll
			if p.Cursor < p.Offset {
				p.Offset = p.Cursor
			} else if p.Cursor >= p.Offset+maxListLines {
				p.Offset = p.Cursor - maxListLines + 1
			}
			maxOffset := max(0, len(p.Rows)-maxListLines)
			if p.Offset > maxOffset {
				p.Offset = maxOffset
			}
			if p.Offset < 0 {
				p.Offset = 0
			}

			endIdx := min(len(p.Rows), p.Offset+maxListLines)
			for i := p.Offset; i < endIdx; i++ {
				row := p.Rows[i]
				group := p.FilteredGroups[row.GroupIndex]
				isSelected := (i == p.Cursor)

				cursorStr := "  "
				if isSelected {
					cursorStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary).Render("> ")
				}

				if row.IsGroupHeader {
					// Group Header Row
					arrow := "▼ "
					if !group.Expanded {
						arrow = "▶ "
					}
					arrowStr := lipgloss.NewStyle().Foreground(styles.ColorPrimary).Render(arrow)

					nameStyle := lipgloss.NewStyle().Bold(true)
					if isSelected {
						nameStyle = nameStyle.Foreground(styles.ColorWhite)
					} else {
						nameStyle = nameStyle.Foreground(textFg)
					}
					nameStr := nameStyle.Render(group.Name)

					countInfo := lipgloss.NewStyle().Foreground(dimFg).
						Render(fmt.Sprintf("(%d paths, %d active)", len(group.Executables), group.TotalInst))

					hintStr := lipgloss.NewStyle().Foreground(dimFg).Render("[enter: add group]")
					gap := p.Width - lipgloss.Width(cursorStr+arrowStr+nameStr+" "+countInfo) - lipgloss.Width(hintStr) - 1
					if gap < 2 {
						gap = 2
					}

					line := fmt.Sprintf("%s%s%s %s%s%s",
						cursorStr, arrowStr, nameStr, countInfo, strings.Repeat(" ", gap), hintStr)
					lines = append(lines, line)

				} else {
					// Executable Item Row
					item := group.Executables[row.ExecIndex]

					checkStr := "[x] "
					if item.Selected {
						checkStr = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorSuccess).Render("[x] ")
					} else {
						checkStr = lipgloss.NewStyle().Foreground(dimFg).Render("[ ] ")
					}

					pathStyle := lipgloss.NewStyle()
					if isSelected {
						pathStyle = pathStyle.Bold(true).Foreground(styles.ColorWhite)
					} else if item.Selected {
						pathStyle = pathStyle.Foreground(textFg)
					} else {
						pathStyle = pathStyle.Foreground(dimFg)
					}
					pathStr := pathStyle.Render(item.Executable)

					instStr := lipgloss.NewStyle().Foreground(styles.ColorSuccess).Render(fmt.Sprintf("(%d)", item.Instances))

					cmdSnippet := item.Command
					if len(cmdSnippet) > 35 {
						cmdSnippet = cmdSnippet[:32] + "..."
					}
					cmdStr := lipgloss.NewStyle().Foreground(dimFg).Render(cmdSnippet)

					line := fmt.Sprintf("%s    %s%s %s  %s", cursorStr, checkStr, pathStr, instStr, cmdStr)
					lines = append(lines, line)
				}
			}
		}
	} else {
		// Custom Path Mode: input field
		lines = append(lines, "")
		prompt := lipgloss.NewStyle().Foreground(dimFg).Render("Enter Executable Path or Command Name:")
		lines = append(lines, fmt.Sprintf("  %s", prompt))

		inputBox := lipgloss.NewStyle().
			Foreground(styles.ColorWhite).
			Bold(true).
			Render("  > " + p.CustomInput + "_")
		lines = append(lines, inputBox)

		hint := lipgloss.NewStyle().Foreground(dimFg).Render("  (e.g. /home/i4N/.gemini/bin/agy, /usr/bin/curl, or google-chrome)")
		lines = append(lines, hint)
	}

	// 3. Bottom controls
	bottomHint := lipgloss.NewStyle().Foreground(dimFg).
		Render("  [space] toggle  [enter] add  [tab] mode  [ctrl+v/paste: paste]  [esc] cancel")
	for len(lines) < availableHeight-1 {
		lines = append(lines, "")
	}
	lines = append(lines, bottomHint)

	content := strings.Join(lines, "\n")
	boxStyle := lipgloss.NewStyle().
		Width(p.Width).
		Height(availableHeight)

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, boxStyle.Render(content))
}
