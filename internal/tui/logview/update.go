package logview

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the log viewer.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case message.StatusMessage:
		m.statusMsg = msg.Text
		if msg.Duration > 0 {
			m.statusExpiry = time.Now().Add(msg.Duration)
			return m, tea.Tick(msg.Duration, func(time.Time) tea.Msg {
				return message.StatusMessageExpired{}
			})
		}
		return m, nil

	case message.StatusMessageExpired:
		if !m.statusExpiry.IsZero() && time.Now().After(m.statusExpiry) {
			m.statusMsg = ""
		}
		return m, nil
	default:
	}

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
	// Two-stage Esc: first clears search, second propagates to root for PopView.
	if msg.Type == tea.KeyEsc && m.HasActiveSearch() {
		m.searchTerm = ""
		m.matches = nil
		m.matchIdx = -1
		for i := range m.entries {
			m.entries[i].Expanded = false
		}
		m.refreshViewportContent()
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Search):
		m.searchActive = true
		m.searchInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.FollowToggle):
		m.followMode = !m.followMode
		if m.followMode {
			m.cursor = max(0, len(m.entries)-1)
			m.refreshViewportContent()
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
	// Expand/collapse current entry.
	case key.Matches(msg, m.keys.Open):
		if m.cursor >= 0 && m.cursor < len(m.entries) && m.entries[m.cursor].IsMultiLine() {
			m.entries[m.cursor].Expanded = !m.entries[m.cursor].Expanded
			m.refreshViewportContent()
			m.scrollToCursor()
		}
		return m, nil

	// Expand all multi-line entries.
	case key.Matches(msg, m.keys.ExpandAll):
		for i := range m.entries {
			if m.entries[i].IsMultiLine() {
				m.entries[i].Expanded = true
			}
		}
		m.refreshViewportContent()
		return m, nil

	// Collapse all entries.
	case key.Matches(msg, m.keys.CollapseAll):
		for i := range m.entries {
			m.entries[i].Expanded = false
		}
		m.refreshViewportContent()
		return m, nil

	// Entry-level cursor navigation (Up/Down).
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.followMode = false
			m.refreshViewportContent()
			m.scrollToCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.followMode = false
			m.refreshViewportContent()
			m.scrollToCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.GotoTop):
		m.cursor = 0
		m.followMode = false
		m.refreshViewportContent()
		m.viewport.GotoTop()
		return m, nil

	case key.Matches(msg, m.keys.GotoBottom):
		m.cursor = max(0, len(m.entries)-1)
		m.followMode = false
		m.refreshViewportContent()
		m.viewport.GotoBottom()
		return m, nil
	}

	// HalfPageUp/Down and other keys go directly to viewport (raw line scrolling).
	// These don't move the cursor.
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
			m.searchInput.Reset()
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
