package fitness

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the fitness detail view.
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

	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward to viewport for scrolling.
	if m.focus == FocusReport {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return message.PopView{} }

	case key.Matches(msg, m.keys.Dismiss):
		return m.handleDismiss()

	case key.Matches(msg, m.keys.Sources):
		return m.handleJumpToSources()

	case key.Matches(msg, m.keys.Chat):
		return m, func() tea.Msg { return message.ChatPanelToggled{} }

	case key.Matches(msg, m.keys.TabForward):
		if m.focus == FocusReport {
			m.focus = FocusChat
		} else {
			m.focus = FocusReport
		}
		return m, nil

	case key.Matches(msg, m.keys.Open):
		return m.handleToggleEvidence()

	case key.Matches(msg, m.keys.Up):
		if m.focus == FocusReport {
			if m.selectedEvidence > 0 {
				m.selectedEvidence--
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.focus == FocusReport {
			if len(m.evidence) > 0 && m.selectedEvidence < len(m.evidence)-1 {
				m.selectedEvidence++
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case key.Matches(msg, m.keys.HalfPageUp), key.Matches(msg, m.keys.HalfPageDown):
		if m.focus == FocusReport {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

// handleDismiss emits a DismissFinished message to archive the report.
func (m Model) handleDismiss() (Model, tea.Cmd) {
	if m.report == nil {
		return m, nil
	}
	id := m.report.ID
	return m, func() tea.Msg {
		return message.DismissFinished{ReportID: id, Err: nil}
	}
}

// handleJumpToSources emits a JumpToSources message with the report's source name.
func (m Model) handleJumpToSources() (Model, tea.Cmd) {
	if m.report == nil {
		return m, nil
	}
	name := m.report.SourceName
	return m, func() tea.Msg {
		return message.JumpToSources{SourceName: name}
	}
}

// handleToggleEvidence toggles the expanded state of the evidence group at
// the current cursor position and rebuilds the viewport content.
func (m Model) handleToggleEvidence() (Model, tea.Cmd) {
	if len(m.evidence) == 0 {
		return m, nil
	}
	if m.selectedEvidence < 0 || m.selectedEvidence >= len(m.evidence) {
		return m, nil
	}

	m.evidence[m.selectedEvidence].Expanded = !m.evidence[m.selectedEvidence].Expanded
	m.viewport.SetContent(m.renderViewportContent())
	return m, nil
}
