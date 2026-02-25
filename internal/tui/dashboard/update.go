package dashboard

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the dashboard.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// When filter bar is active, route input to the text input.
	if m.filterActive {
		return m.updateFilter(msg)
	}

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

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width
		m.viewport.Height = m.viewportHeight()
		m.updateContent()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.HalfPageDown):
		jump := m.viewport.Height / 2
		m.cursor += jump
		if m.cursor >= len(m.filtered) {
			m.cursor = max(0, len(m.filtered)-1)
		}
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.HalfPageUp):
		jump := m.viewport.Height / 2
		m.cursor -= jump
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.GotoTop):
		m.cursor = 0
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.GotoBottom):
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		m.updateContent()
		return m, nil

	case key.Matches(msg, m.keys.Open):
		item := m.SelectedItem()
		if item == nil {
			return m, nil
		}
		if item.IsFitnessReport() {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewFitnessDetail}
			}
		}
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewProposalDetail}
		}

	case key.Matches(msg, m.keys.Sort):
		m.CycleSortOrder()
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		m.filterActive = true
		m.filterInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.Sources):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewSourceManager}
		}

	case key.Matches(msg, m.keys.Pipeline):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewPipelineMonitor}
		}

	case key.Matches(msg, m.keys.Approve):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "approve"}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Reject):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "reject"}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Defer):
		if m.SelectedProposal() != nil {
			return m, func() tea.Msg {
				return message.PushView{View: message.ViewProposalDetail, Action: "defer"}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) updateFilter(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.filterActive = false
			m.filterInput.Blur()
			m.filterText = ""
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.filterActive = false
			m.filterInput.Blur()
			m.filterText = m.filterInput.Value()
			m.applyFilter()
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}
