package ops

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the Operations view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case message.OpsDataRefreshed:
		m.UpdateStats(msg.Stats)
		return m, nil
	}

	// Key input.
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		return m.handleKey(msg)
	}

	// Forward to viewport for scroll events.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	eventCount := len(m.stats.RecentEvents)

	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < eventCount-1 {
			m.cursor++
			m.refreshViewport()
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.refreshViewport()
		}
		return m, nil

	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return message.PopView{} }
	}

	return m, nil
}
