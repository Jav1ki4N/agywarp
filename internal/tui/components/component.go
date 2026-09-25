package components

import tea "charm.land/bubbletea/v2"

type ComponentBase struct {
	Width   int
	Height  int
	Focused bool
}

type Component interface {
	Init() tea.Cmd
	Update(tea.Msg) tea.Cmd
	Render() string
}
