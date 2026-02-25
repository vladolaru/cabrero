package dashboard

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the dashboard.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, m.viewportHeight(msg.Width, msg.Height))
		return m, nil

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

	case tea.KeyPressMsg:
		// While filter is active, route all keys to the list.
		if m.list.SettingFilter() {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		return m.handleKey(msg)
	}

	// Forward all other messages (mouse, spinner ticks, etc.) to list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
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
		cmd := m.CycleSortOrder()
		return m, cmd

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

	case key.Matches(msg, m.keys.Sources):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewSourceManager}
		}

	case key.Matches(msg, m.keys.Pipeline):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewPipelineMonitor}
		}
	}

	// Navigation (up/down/half-page/goto) handled by list.
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
