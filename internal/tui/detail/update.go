package detail

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the detail view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case components.ConfirmResult:
		return m.handleConfirmResult(msg)

	case components.RevisionChoice:
		return m.handleRevisionChoice(msg)

	case message.BlendFinished:
		return m.handleBlendFinished(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward spinner ticks when blending.
	if m.applyState == ApplyBlending {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Forward to active viewport.
	if m.focus == FocusProposal {
		var cmd tea.Cmd
		m.diffViewport, cmd = m.diffViewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// When confirming, route to confirm component.
	if m.applyState == ApplyConfirming {
		if m.HasRevision() {
			var cmd tea.Cmd
			m.revConfirm, cmd = m.revConfirm.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	}

	// When reviewing blend result, handle confirmation.
	if m.applyState == ApplyReviewing {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	}

	switch {
	case key.Matches(msg, m.keys.TabForward):
		if m.focus == FocusProposal {
			m.focus = FocusChat
		} else {
			m.focus = FocusProposal
		}
		return m, nil

	case key.Matches(msg, m.keys.Approve):
		return m.startApprove()

	case key.Matches(msg, m.keys.Reject):
		return m.startReject()

	case key.Matches(msg, m.keys.Defer):
		return m.startDefer()

	case key.Matches(msg, m.keys.UseRevision):
		// Only meaningful if revision exists — handled by chat model.
		return m, nil

	case key.Matches(msg, m.keys.Up), key.Matches(msg, m.keys.Down),
		key.Matches(msg, m.keys.HalfPageUp), key.Matches(msg, m.keys.HalfPageDown):
		if m.focus == FocusProposal {
			var cmd tea.Cmd
			m.diffViewport, cmd = m.diffViewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

func (m Model) startApprove() (Model, tea.Cmd) {
	if m.proposal == nil {
		return m, nil
	}
	m.applyState = ApplyConfirming

	if m.HasRevision() {
		m.revConfirm = components.NewRevisionConfirm("Apply this change?")
		return m, nil
	}

	m.confirm = components.NewConfirm("Apply this change?")
	return m, nil
}

func (m Model) handleConfirmResult(msg components.ConfirmResult) (Model, tea.Cmd) {
	if !msg.Confirmed {
		m.applyState = ApplyIdle
		return m, nil
	}

	switch m.applyState {
	case ApplyConfirming:
		// Start blending.
		m.applyState = ApplyBlending
		id := m.proposal.Proposal.ID
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg {
				return message.ApproveStarted{ProposalID: id}
			},
		)
	case ApplyReviewing:
		// Confirm apply after blend review.
		id := m.proposal.Proposal.ID
		m.applyState = ApplyDone
		return m, func() tea.Msg {
			return message.ApplyConfirmed{ProposalID: id}
		}
	}

	return m, nil
}

func (m Model) handleRevisionChoice(msg components.RevisionChoice) (Model, tea.Cmd) {
	switch msg.Choice {
	case "cancel":
		m.applyState = ApplyIdle
		return m, nil
	case "original", "revision":
		m.applyState = ApplyBlending
		id := m.proposal.Proposal.ID
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg {
				return message.ApproveStarted{ProposalID: id}
			},
		)
	}
	return m, nil
}

func (m Model) handleBlendFinished(msg message.BlendFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.applyState = ApplyIdle
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Blend failed: " + msg.Err.Error()}
		}
	}

	m.applyState = ApplyReviewing
	m.blendResult = &msg.BeforeAfterDiff
	m.confirm = components.NewConfirm("Commit this change?")
	return m, nil
}

func (m Model) startReject() (Model, tea.Cmd) {
	if m.proposal == nil {
		return m, nil
	}
	id := m.proposal.Proposal.ID
	return m, func() tea.Msg {
		return message.RejectFinished{ProposalID: id}
	}
}

func (m Model) startDefer() (Model, tea.Cmd) {
	if m.proposal == nil {
		return m, nil
	}
	id := m.proposal.Proposal.ID
	return m, func() tea.Msg {
		return message.DeferFinished{ProposalID: id}
	}
}
