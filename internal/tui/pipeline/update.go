package pipeline

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the pipeline monitor.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Handle ConfirmResult FIRST (before checking Active).
	if result, ok := msg.(components.ConfirmResult); ok {
		return m.handleConfirmResult(result)
	}

	// Route to confirm dialog when active.
	if m.confirm.Active {
		return m.updateConfirm(msg)
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
		if m.cursor < len(m.runs)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.Open):
		// Toggle inline expansion.
		if m.expandedIdx == m.cursor {
			m.expandedIdx = -1
		} else {
			m.expandedIdx = m.cursor
		}
		return m, nil

	case key.Matches(msg, m.keys.Retry):
		run := m.SelectedRun()
		if run != nil && run.Status == "error" {
			if m.config.Confirmations.RetryRequiresConfirm {
				m.confirm = components.NewConfirm("Retry session " + truncateID(run.SessionID) + "?")
				return m, nil
			}
			// Skip confirmation — retry immediately.
			sessionID := run.SessionID
			m.retrying = sessionID
			return m, func() tea.Msg {
				return message.RetryRunStarted{SessionID: sessionID}
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.LogView):
		return m, func() tea.Msg {
			return message.PushView{View: message.ViewLogViewer}
		}

	case key.Matches(msg, m.keys.Refresh):
		return m, nil
	}

	return m, nil
}

func (m Model) updateConfirm(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmResult(result components.ConfirmResult) (Model, tea.Cmd) {
	if !result.Confirmed {
		return m, nil
	}

	run := m.SelectedRun()
	if run != nil {
		sessionID := run.SessionID
		m.retrying = sessionID
		return m, func() tea.Msg {
			return message.RetryRunStarted{SessionID: sessionID}
		}
	}

	return m, nil
}

// truncateID returns the first 8 characters of an ID string.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
