package tui

import (
	"agywarp/internal/tui/pages"

	tea "charm.land/bubbletea/v2"
)

// model is top layer model that interacts with BubbleTEA, a collection of states
type model struct {
	current_page pages.Page
}

// newModel creates a new model instance
func newModel() model {
	return model{
		current_page: &pages.Home{},
	}
}

// Init operation will launch the TUI
func (m model) Init() tea.Cmd {
	return tea.Batch(
		m.current_page.Init(),
		tea.RequestBackgroundColor,
	)
}

// Update updates model's state based on tea.Msg and returns a new state
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.current_page, cmd = m.current_page.Update(msg)
	return m, cmd
}

// Cleanup releases UI resources. Tunnel state is changed only by Network Card.
func (m model) Cleanup() {
	if cleaner, ok := m.current_page.(interface{ Cleanup() }); ok {
		cleaner.Cleanup()
	}
}

// View renders the model's content and shows them on terminal
func (m model) View() tea.View {
	v := tea.NewView(m.current_page.Render())
	v.AltScreen = true
	return v
}
