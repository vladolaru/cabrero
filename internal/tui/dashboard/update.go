package dashboard

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the dashboard.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// When filter bar is active, route input to the text input.
	if m.filterActive {
		return m.updateFilter(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.GotoTop):
		m.cursor = 0
		return m, nil

	case key.Matches(msg, m.keys.GotoBottom):
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
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
	if msg, ok := msg.(tea.KeyMsg); ok {
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
