package tui

import (
	"agywarp/internal/tui/pages"
	tea "charm.land/bubbletea/v2"
	"os"
)

func Run() error {
	check, err := runPreflight(os.Stdout)
	if err != nil {
		return err
	}
	m := newModel()
	if home, ok := m.current_page.(*pages.Home); ok {
		home.InitialTunnelActive = check.Active
	}
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}
