package logview

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages for the log viewer.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.searchActive {
		return m.updateSearch(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward to viewport for scrolling.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Search):
		m.searchActive = true
		m.searchInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.FollowToggle):
		m.followMode = !m.followMode
		if m.followMode {
			m.viewport.GotoBottom()
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchNext):
		if len(m.matches) > 0 {
			next := (m.matchIdx + 1) % len(m.matches)
			m.gotoMatch(next)
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchPrev):
		if len(m.matches) > 0 {
			prev := m.matchIdx - 1
			if prev < 0 {
				prev = len(m.matches) - 1
			}
			m.gotoMatch(prev)
		}
		return m, nil
	}

	// Any manual scroll disables follow mode.
	if msg.Type == tea.KeyUp || msg.Type == tea.KeyDown ||
		msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown {
		m.followMode = false
	}

	// Forward to viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) updateSearch(msg tea.Msg) (Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(kmsg, key.NewBinding(key.WithKeys("esc"))):
			m.searchActive = false
			m.searchInput.Blur()
			return m, nil
		case key.Matches(kmsg, key.NewBinding(key.WithKeys("enter"))):
			m.searchActive = false
			m.searchInput.Blur()
			m.searchTerm = m.searchInput.Value()
			m.performSearch()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}
