package pages

import tea "charm.land/bubbletea/v2"

// PageBase is a set of states shared by pages
type PageBase struct {
	Width  int
	Height int
	cursor int
}

// Page defines the func a Page struct must impl
type Page interface {
	Init() tea.Cmd
	Update(msg tea.Msg) (Page, tea.Cmd)
	Render() string
}
