package sources

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the source manager.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Data messages are handled first regardless of sub-view state,
	// since they are results of previous actions, not user input.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case message.ToggleApproachFinished:
		return m.handleToggleFinished(msg)

	case message.SetOwnershipFinished:
		return m.handleOwnershipFinished(msg)

	case message.RollbackFinished:
		return m.handleRollbackFinished(msg)
	}

	// When a confirmation prompt is active, route input there.
	if m.confirm.Active {
		return m.updateConfirm(msg)
	}

	// When detail sub-view is open, route input there.
	if m.detailOpen {
		return m.updateDetail(msg)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.flatItems)-1 {
			m.cursor++
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case key.Matches(msg, m.keys.Left):
		// Collapse the group at or containing the cursor.
		gi := m.cursorGroupIdx()
		if gi >= 0 && !m.groups[gi].Collapsed {
			m.groups[gi].Collapsed = true
			m.rebuildFlatItems()
		}
		return m, nil

	case key.Matches(msg, m.keys.Right):
		// Expand the group at or containing the cursor.
		gi := m.cursorGroupIdx()
		if gi >= 0 && m.groups[gi].Collapsed {
			m.groups[gi].Collapsed = false
			m.rebuildFlatItems()
		}
		return m, nil

	case key.Matches(msg, m.keys.Open):
		return m.handleOpen()

	case key.Matches(msg, m.keys.ToggleApproach):
		return m.handleToggleApproach()

	case key.Matches(msg, m.keys.SetOwnership):
		return m.handleSetOwnership()

	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return message.PopView{} }
	}

	return m, nil
}

// handleOpen opens the detail sub-view for the selected source, or triggers
// classification for unclassified sources.
func (m Model) handleOpen() (Model, tea.Cmd) {
	s := m.SelectedSource()
	if s == nil {
		return m, nil
	}

	if s.Ownership == "" {
		// Unclassified source — trigger classification prompt.
		name := s.Name
		return m, func() tea.Msg {
			return message.ClassifyFinished{SourceName: name}
		}
	}

	// Open detail sub-view.
	m.detailOpen = true
	src := *s
	m.detailSource = &src
	// Changes would be loaded asynchronously in real usage;
	// for now we store whatever was previously set.
	return m, nil
}

// handleToggleApproach initiates the approach toggle with confirmation.
func (m Model) handleToggleApproach() (Model, tea.Cmd) {
	s := m.SelectedSource()
	if s == nil || s.Ownership == "" {
		return m, nil
	}

	newApproach := "evaluate"
	if s.Approach == "evaluate" {
		newApproach = "iterate"
	}

	m.confirmState = ConfirmToggleApproach
	m.confirm = components.NewConfirm("Toggle " + s.Name + " to " + newApproach + "?")
	return m, nil
}

// handleSetOwnership prompts for ownership change.
func (m Model) handleSetOwnership() (Model, tea.Cmd) {
	s := m.SelectedSource()
	if s == nil {
		return m, nil
	}

	m.confirmState = ConfirmSetOwnership
	m.confirm = components.NewConfirm("Set " + s.Name + " ownership: [m]ine or [n]ot mine?")
	return m, nil
}

// updateConfirm routes messages to the confirmation sub-component.
func (m Model) updateConfirm(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)

	// Check if ConfirmResult was emitted.
	if cmd != nil {
		resultMsg := cmd()
		if result, ok := resultMsg.(components.ConfirmResult); ok {
			return m.handleConfirmResult(result)
		}
		// Not a ConfirmResult, pass cmd through.
		return m, cmd
	}

	return m, nil
}

func (m Model) handleConfirmResult(result components.ConfirmResult) (Model, tea.Cmd) {
	state := m.confirmState
	m.confirmState = ConfirmNone

	if !result.Confirmed {
		return m, nil
	}

	s := m.SelectedSource()

	switch state {
	case ConfirmToggleApproach:
		if s == nil {
			return m, nil
		}
		newApproach := "evaluate"
		if s.Approach == "evaluate" {
			newApproach = "iterate"
		}
		name := s.Name
		approach := newApproach
		return m, func() tea.Msg {
			return message.ToggleApproachFinished{SourceName: name, NewApproach: approach}
		}

	case ConfirmSetOwnership:
		// For now, this emits a finished message. In real usage, a sub-prompt
		// would ask for [m]ine / [n]ot mine. Simplified for this phase.
		if s == nil {
			return m, nil
		}
		name := s.Name
		return m, func() tea.Msg {
			return message.SetOwnershipFinished{SourceName: name, NewOwnership: "mine"}
		}

	case ConfirmRollback:
		if m.detailSource == nil || len(m.changes) == 0 {
			return m, nil
		}
		changeID := m.changes[0].ID
		return m, func() tea.Msg {
			return message.RollbackFinished{ChangeID: changeID}
		}
	}

	return m, nil
}

// updateDetail handles keys when the detail sub-view is open.
func (m Model) updateDetail(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, m.keys.Back):
			m.detailOpen = false
			m.detailSource = nil
			m.changes = nil
			return m, nil

		case key.Matches(msg, m.keys.Rollback):
			if len(m.changes) == 0 {
				return m, nil
			}
			if m.config.Confirmations.RollbackRequiresConfirm {
				m.confirmState = ConfirmRollback
				m.confirm = components.NewConfirm("Rollback change " + m.changes[0].ID + "?")
				return m, nil
			}
			// No confirmation needed — emit directly.
			changeID := m.changes[0].ID
			return m, func() tea.Msg {
				return message.RollbackFinished{ChangeID: changeID}
			}
		}
	}
	return m, nil
}

// handleToggleFinished processes the result of an approach toggle.
func (m Model) handleToggleFinished(msg message.ToggleApproachFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Toggle failed: " + msg.Err.Error()}
		}
	}
	// Update the source in our local state.
	for gi := range m.groups {
		for si := range m.groups[gi].Sources {
			if m.groups[gi].Sources[si].Name == msg.SourceName {
				m.groups[gi].Sources[si].Approach = msg.NewApproach
				return m, nil
			}
		}
	}
	return m, nil
}

// handleOwnershipFinished processes the result of an ownership change.
func (m Model) handleOwnershipFinished(msg message.SetOwnershipFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Ownership change failed: " + msg.Err.Error()}
		}
	}
	for gi := range m.groups {
		for si := range m.groups[gi].Sources {
			if m.groups[gi].Sources[si].Name == msg.SourceName {
				m.groups[gi].Sources[si].Ownership = msg.NewOwnership
				return m, nil
			}
		}
	}
	return m, nil
}

// handleRollbackFinished processes the result of a rollback.
func (m Model) handleRollbackFinished(msg message.RollbackFinished) (Model, tea.Cmd) {
	if msg.Err != nil {
		return m, func() tea.Msg {
			return message.StatusMessage{Text: "Rollback failed: " + msg.Err.Error()}
		}
	}
	// Remove the rolled-back change from the list.
	for i, c := range m.changes {
		if c.ID == msg.ChangeID {
			m.changes = append(m.changes[:i], m.changes[i+1:]...)
			break
		}
	}
	return m, nil
}

// cursorGroupIdx returns the group index for the current cursor position.
// If the cursor is on a header, returns that group index.
// If on a source, returns the containing group index.
func (m Model) cursorGroupIdx() int {
	if m.cursor < 0 || m.cursor >= len(m.flatItems) {
		return -1
	}
	return m.flatItems[m.cursor].groupIdx
}
