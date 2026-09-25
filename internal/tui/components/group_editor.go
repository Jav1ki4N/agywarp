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

type GroupSavedMsg struct {
	Profile   process.Profile
	IsEditing bool
}

type GroupEditor struct {
	ComponentBase
	Open      bool
	IsEditing bool
	EditingID string

	NameInput   string
	Paths       []string
	PathInput   string
	ActiveField int // 0: Name, 1: Paths list, 2: Add Path input
	PathCursor  int // Cursor when navigating Paths list

	ErrorMsg          string
	lastAttemptedPath string
	RunningProcesses  []process.RunningProcess

	// Dynamic grays
	DelimFg color.Color
	TitleFg color.Color
	DimFg   color.Color
	TextFg  color.Color
}

func NewGroupEditor() GroupEditor {
	return GroupEditor{
		ComponentBase: ComponentBase{Height: 10},
		Open:          false,
		ActiveField:   0,
		DelimFg:       styles.ColorDarkGray,
		TitleFg:       styles.ColorDimGray,
		DimFg:         styles.ColorDimGray,
		TextFg:        styles.ColorLightGray,
	}
}

func (e *GroupEditor) SetRunningProcesses(procs []process.RunningProcess) {
	e.RunningProcesses = procs
}

func (e *GroupEditor) OpenCreate() {
	e.Open = true
	e.IsEditing = false
	e.EditingID = ""
	e.NameInput = ""
	e.Paths = nil
	e.PathInput = ""
	e.ErrorMsg = ""
	e.lastAttemptedPath = ""
	e.ActiveField = 0
	e.PathCursor = 0
}

func (e *GroupEditor) OpenEdit(p process.Profile) {
	e.Open = true
	e.IsEditing = true
	e.EditingID = p.ID
	e.NameInput = p.Label
	e.Paths = nil
	for _, m := range p.Matchers {
		e.Paths = append(e.Paths, m.Pattern)
	}
	e.PathInput = ""
	e.ErrorMsg = ""
	e.lastAttemptedPath = ""
	e.ActiveField = 0
	e.PathCursor = 0
}

func (e *GroupEditor) Init() tea.Cmd {
	return nil
}

func (e *GroupEditor) handlePaste(content string) tea.Cmd {
	content = strings.ReplaceAll(content, "\r", "")
	if e.ActiveField == 0 {
		cleaned := strings.ReplaceAll(content, "\n", " ")
		cleaned = strings.TrimSpace(cleaned)
		e.NameInput += cleaned
		return nil
	}

	// ActiveField is 1 or 2
	lines := strings.Split(content, "\n")
	var nonEmpty []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}

	if len(nonEmpty) > 1 {
		// Multi-line paste: validate each line and add
		var cmds []tea.Cmd
		for _, line := range nonEmpty {
			res, err := process.ValidatePath(line, e.RunningProcesses)
			if err != nil {
				e.ErrorMsg = fmt.Sprintf("Some paths not found in system (e.g. %s)", line)
				p := line
				cmds = append(cmds, func() tea.Msg {
					return LogMsg{
						Level:   "ERR",
						Message: fmt.Sprintf("Path does not exist in system: %s (%v)", p, err),
					}
				})
			} else {
				pathToAdd := line
				if strings.HasPrefix(line, "~") && res.ResolvedPath != "" {
					pathToAdd = res.ResolvedPath
				}
				e.Paths = append(e.Paths, pathToAdd)
				p := pathToAdd
				d := res.Details
				cmds = append(cmds, func() tea.Msg {
					return LogMsg{
						Level:   "OK",
						Message: fmt.Sprintf("Validated and added path: %s (%s)", p, d),
					}
				})
			}
		}
		e.PathInput = ""
		e.ActiveField = 1
		if len(e.Paths) > 0 {
			e.PathCursor = len(e.Paths) - 1
		}
		return tea.Batch(cmds...)
	} else if len(nonEmpty) == 1 {
		// Single-line paste: append to PathInput
		e.ActiveField = 2
		e.PathInput += nonEmpty[0]
		e.ErrorMsg = ""
		e.lastAttemptedPath = ""
	}
	return nil
}

func (e *GroupEditor) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		e.DelimFg = styles.ElevateColor(msg, 35)
		e.TitleFg = styles.ElevateColor(msg, 75)
		e.DimFg = styles.ElevateColor(msg, 55)
		e.TextFg = styles.ElevateColor(msg, 130)
		return nil

	case tea.PasteMsg:
		if !e.Open {
			return nil
		}
		return e.handlePaste(msg.Content)
	}

	if !e.Open {
		return nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		key := msg.String()

		// 1. Global modal keys (apply to all fields)
		switch key {
		case "esc":
			e.Open = false
			return nil

		case "ctrl+s":
			return e.submit()

		case "ctrl+v":
			if text := clipboard.ReadText(); text != "" {
				return e.handlePaste(text)
			}
			return nil

		case "tab":
			// Cycle active field: 0 (Name) -> 1 (Paths list, if not empty) -> 2 (Add Path input) -> 0
			if len(e.Paths) > 0 {
				e.ActiveField = (e.ActiveField + 1) % 3
			} else {
				if e.ActiveField == 0 {
					e.ActiveField = 2
				} else {
					e.ActiveField = 0
				}
			}
			return nil

		case "shift+tab", "backtab":
			if len(e.Paths) > 0 {
				e.ActiveField = (e.ActiveField + 2) % 3
			} else {
				if e.ActiveField == 0 {
					e.ActiveField = 2
				} else {
					e.ActiveField = 0
				}
			}
			return nil
		}

		// 2. Field 1: Paths List navigation mode
		if e.ActiveField == 1 {
			switch key {
			case "up", "k":
				if e.PathCursor > 0 {
					e.PathCursor--
				} else {
					e.ActiveField = 0
				}
				return nil

			case "down", "j":
				if e.PathCursor < len(e.Paths)-1 {
					e.PathCursor++
				} else {
					e.ActiveField = 2
				}
				return nil

			case "d", "delete":
				if len(e.Paths) > 0 && e.PathCursor < len(e.Paths) {
					e.Paths = append(e.Paths[:e.PathCursor], e.Paths[e.PathCursor+1:]...)
					if e.PathCursor >= len(e.Paths) && e.PathCursor > 0 {
						e.PathCursor = len(e.Paths) - 1
					}
					if len(e.Paths) == 0 {
						e.ActiveField = 2
					}
				}
				return nil

			case "enter":
				// Pressing enter on paths list moves focus down to Add Path input
				e.ActiveField = 2
				return nil

			default:
				// If user starts typing while focused on list, automatically route to Add Path input
				k := msg.Key()
				text := k.Text
				if text == "" && (k.Code == tea.KeySpace || key == "space") {
					text = " "
				}
				if text != "" {
					e.ActiveField = 2
					e.ErrorMsg = ""
					e.lastAttemptedPath = ""
					if len(text) > 1 && (strings.Contains(text, "\n") || strings.Contains(text, "\r")) {
						return e.handlePaste(text)
					}
					e.PathInput += text
					return nil
				}
			}
			return nil
		}

		// 3. Text Input Fields (ActiveField == 0: Name, ActiveField == 2: Path)
		switch key {
		case "up":
			// Arrow key up: navigate to previous field
			if e.ActiveField == 2 {
				if len(e.Paths) > 0 {
					e.ActiveField = 1
					e.PathCursor = len(e.Paths) - 1
				} else {
					e.ActiveField = 0
				}
			}
			return nil

		case "down":
			// Arrow key down: navigate to next field
			if e.ActiveField == 0 {
				if len(e.Paths) > 0 {
					e.ActiveField = 1
					e.PathCursor = 0
				} else {
					e.ActiveField = 2
				}
			}
			return nil

		case "enter":
			if e.ActiveField == 0 {
				// From Name field, jump to Add Path input
				e.ActiveField = 2
				return nil
			} else if e.ActiveField == 2 {
				// In Add Path input: validate path exists in system before appending
				trimmed := strings.TrimSpace(e.PathInput)
				if trimmed != "" {
					res, err := process.ValidatePath(trimmed, e.RunningProcesses)
					if err != nil {
						// If user presses Enter again on the same unverified path, force add
						if e.ErrorMsg != "" && e.lastAttemptedPath == trimmed {
							pathToAdd := trimmed
							if res.ResolvedPath != "" {
								pathToAdd = res.ResolvedPath
							}
							e.Paths = append(e.Paths, pathToAdd)
							e.PathInput = ""
							e.ErrorMsg = ""
							e.lastAttemptedPath = ""
							return func() tea.Msg {
								return LogMsg{
									Level:   "WARN",
									Message: fmt.Sprintf("Force added unverified path: %s", pathToAdd),
								}
							}
						}

						e.ErrorMsg = fmt.Sprintf("Path not found: %s (press Enter again to force add)", trimmed)
						e.lastAttemptedPath = trimmed
						return func() tea.Msg {
							return LogMsg{
								Level:   "ERR",
								Message: fmt.Sprintf("Path does not exist in system: %s (%v)", trimmed, err),
							}
						}
					}

					// Validated successfully
					pathToAdd := trimmed
					if strings.HasPrefix(trimmed, "~") && res.ResolvedPath != "" {
						pathToAdd = res.ResolvedPath
					}
					e.Paths = append(e.Paths, pathToAdd)
					e.PathInput = ""
					e.ErrorMsg = ""
					e.lastAttemptedPath = ""
					return func() tea.Msg {
						return LogMsg{
							Level:   "OK",
							Message: fmt.Sprintf("Validated and added path: %s (%s)", pathToAdd, res.Details),
						}
					}
				} else if len(e.Paths) > 0 && strings.TrimSpace(e.NameInput) != "" {
					// Empty enter when name & paths exist -> submit
					return e.submit()
				}
				return nil
			}

		case "backspace":
			e.ErrorMsg = ""
			e.lastAttemptedPath = ""
			if e.ActiveField == 0 {
				r := []rune(e.NameInput)
				if len(r) > 0 {
					e.NameInput = string(r[:len(r)-1])
				}
			} else if e.ActiveField == 2 {
				r := []rune(e.PathInput)
				if len(r) > 0 {
					e.PathInput = string(r[:len(r)-1])
				}
			}

		default:
			// Append printable characters or text into active text field
			k := msg.Key()
			text := k.Text
			if text == "" && (k.Code == tea.KeySpace || key == "space") {
				text = " "
			}
			if text != "" {
				e.ErrorMsg = ""
				e.lastAttemptedPath = ""
				if len(text) > 1 && (strings.Contains(text, "\n") || strings.Contains(text, "\r")) {
					return e.handlePaste(text)
				}
				if e.ActiveField == 0 {
					e.NameInput += text
				} else if e.ActiveField == 2 {
					e.PathInput += text
				}
			}
		}
	}

	return nil
}

func (e *GroupEditor) submit() tea.Cmd {
	name := strings.TrimSpace(e.NameInput)
	if name == "" {
		if len(e.Paths) > 0 {
			name = filepath.Base(e.Paths[0])
		} else {
			name = "Custom Group"
		}
	}

	// Include pending path if typed
	pending := strings.TrimSpace(e.PathInput)
	if pending != "" {
		res, err := process.ValidatePath(pending, e.RunningProcesses)
		if err != nil {
			e.ErrorMsg = fmt.Sprintf("Path not found: %s", pending)
			return func() tea.Msg {
				return LogMsg{
					Level:   "ERR",
					Message: fmt.Sprintf("Cannot save group: path not found: %s (%v)", pending, err),
				}
			}
		}
		pathToAdd := pending
		if strings.HasPrefix(pending, "~") && res.ResolvedPath != "" {
			pathToAdd = res.ResolvedPath
		}
		e.Paths = append(e.Paths, pathToAdd)
		e.PathInput = ""
		e.ErrorMsg = ""
		e.lastAttemptedPath = ""
	}

	if len(e.Paths) == 0 {
		return nil
	}

	var matchers []process.Matcher
	for _, p := range e.Paths {
		kind := process.MatchExecutablePath
		if strings.HasPrefix(p, "*.") || (strings.HasPrefix(p, ".") && !strings.Contains(p, "/")) {
			kind = process.MatchDomainSuffix
			p = strings.TrimPrefix(strings.TrimPrefix(p, "*"), ".")
		} else if !strings.HasPrefix(p, "/") {
			if strings.Contains(p, ".") {
				kind = process.MatchDomain
			} else {
				kind = process.MatchProcessName
			}
		}
		matchers = append(matchers, process.Matcher{
			Kind:    kind,
			Pattern: p,
		})
	}

	var prof process.Profile
	if e.IsEditing && e.EditingID != "" {
		prof = process.Profile{
			ID:       e.EditingID,
			Label:    name,
			Enabled:  true,
			Matchers: matchers,
		}
	} else {
		prof = process.NewGroupProfile(name, matchers, true)
	}

	isEditing := e.IsEditing
	e.Open = false

	return func() tea.Msg {
		return GroupSavedMsg{
			Profile:   prof,
			IsEditing: isEditing,
		}
	}
}

func (e *GroupEditor) Render() string {
	if !e.Open || e.Width <= 0 || e.Height <= 0 {
		return ""
	}

	dimFg := e.DimFg
	if dimFg == nil {
		dimFg = styles.ColorDimGray
	}
	textFg := e.TextFg
	if textFg == nil {
		textFg = styles.ColorLightGray
	}

	// 1. Header
	titleStr := "Add Process Group"
	if e.IsEditing {
		titleStr = "Edit Process Group"
	}

	titleText := "── " + titleStr + " "
	titleWidth := lipgloss.Width(titleText)
	delimStyle := lipgloss.NewStyle().Foreground(e.DelimFg)
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary)

	var headerLine string
	if e.Width > titleWidth {
		headerLine = titleStyle.Render(titleText) + delimStyle.Render(strings.Repeat("─", e.Width-titleWidth))
	} else {
		headerLine = delimStyle.Render(strings.Repeat("─", e.Width))
	}

	availableHeight := e.Height - 1
	if availableHeight <= 0 {
		return headerLine
	}

	var lines []string

	// 2. Field 1: Group Name
	namePrompt := lipgloss.NewStyle()
	if e.ActiveField == 0 {
		namePrompt = namePrompt.Bold(true).Foreground(styles.ColorPrimary)
	} else {
		namePrompt = namePrompt.Foreground(dimFg)
	}
	nameLabel := namePrompt.Render("  Group Name: ")

	cursorMarker := ""
	if e.ActiveField == 0 {
		cursorMarker = "_"
	}
	nameBox := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.ColorWhite).
		Render(e.NameInput + cursorMarker)

	lines = append(lines, fmt.Sprintf("%s[ %s ]", nameLabel, nameBox))
	lines = append(lines, "")

	// 3. Field 2: Paths List
	pathsLabel := lipgloss.NewStyle()
	if e.ActiveField == 1 {
		pathsLabel = pathsLabel.Bold(true).Foreground(styles.ColorPrimary)
	} else {
		pathsLabel = pathsLabel.Foreground(dimFg)
	}
	lines = append(lines, pathsLabel.Render("  Paths / Executables in this group:"))

	if len(e.Paths) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(dimFg).Render("    (no paths yet - add below)"))
	} else {
		// Calculate available lines for paths list (7 fixed lines for name, prompts, hints)
		maxVisiblePaths := availableHeight - 7
		if maxVisiblePaths < 1 {
			maxVisiblePaths = 1
		}

		start := 0
		if e.PathCursor >= maxVisiblePaths {
			start = e.PathCursor - maxVisiblePaths + 1
		}
		end := start + maxVisiblePaths
		if end > len(e.Paths) {
			end = len(e.Paths)
			start = end - maxVisiblePaths
			if start < 0 {
				start = 0
			}
		}

		for i := start; i < end; i++ {
			p := e.Paths[i]
			isSelected := (e.ActiveField == 1 && i == e.PathCursor)
			prefix := "    • "
			if isSelected {
				prefix = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorPrimary).Render("  > • ")
			}

			pathStyle := lipgloss.NewStyle()
			if isSelected {
				pathStyle = pathStyle.Bold(true).Foreground(styles.ColorWhite)
			} else {
				pathStyle = pathStyle.Foreground(textFg)
			}

			delHint := ""
			if isSelected {
				delHint = lipgloss.NewStyle().Foreground(styles.ColorDanger).Render("  [d: remove]")
			}

			lines = append(lines, fmt.Sprintf("%s%s%s", prefix, pathStyle.Render(p), delHint))
		}
	}
	lines = append(lines, "")

	// 4. Field 3: Add Path input
	addPrompt := lipgloss.NewStyle()
	if e.ActiveField == 2 {
		addPrompt = addPrompt.Bold(true).Foreground(styles.ColorPrimary)
	} else {
		addPrompt = addPrompt.Foreground(dimFg)
	}
	addLabel := addPrompt.Render("  + Add Path / Name: ")

	addCursor := ""
	if e.ActiveField == 2 {
		addCursor = "_"
	}
	addBox := lipgloss.NewStyle().
		Foreground(styles.ColorWhite).
		Render(e.PathInput + addCursor)

	lines = append(lines, fmt.Sprintf("%s[ %s ]", addLabel, addBox))
	if e.ErrorMsg != "" {
		errStyle := lipgloss.NewStyle().Bold(true).Foreground(styles.ColorDanger)
		lines = append(lines, errStyle.Render("    ✖ "+e.ErrorMsg))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(dimFg).Render("    (press Enter to append path, e.g. /home/i4N/.gemini/bin/agy or agy)"))
	}

	// 5. Bottom action bar
	bottomHint := lipgloss.NewStyle().Foreground(dimFg).
		Render("  [ctrl+s: Save Group]   [tab: switch field]   [ctrl+v/paste: Paste]   [esc: Cancel]")

	for len(lines) < availableHeight-1 {
		lines = append(lines, "")
	}
	if len(lines) > availableHeight-1 {
		lines = lines[:availableHeight-1]
	}
	lines = append(lines, bottomHint)

	content := strings.Join(lines, "\n")
	boxStyle := lipgloss.NewStyle().
		Width(e.Width).
		Height(availableHeight).
		MaxHeight(availableHeight)

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, boxStyle.Render(content))
}
