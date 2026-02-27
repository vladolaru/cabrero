package sources

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the source manager.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Data messages are handled first regardless of sub-view state,
	// since they are results of previous actions, not user input.
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

	case message.ToggleApproachFinished:
		return m.handleToggleFinished(msg)

	case message.SetOwnershipFinished:
		return m.handleOwnershipFinished(msg)

	case message.RollbackFinished:
		return m.handleRollbackFinished(msg)
	}

	// Handle ConfirmResult messages from the confirm component.
	if result, ok := msg.(components.ConfirmResult); ok {
		return m.handleConfirmResult(result)
	}

	// Ownership prompt uses custom m/n/esc keys, not the generic confirm.
	if m.confirmState == ConfirmSetOwnership {
		return m.updateOwnershipPrompt(msg)
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
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward to viewport for scroll events.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.flatItems)-1 {
			m.cursor++
			m.ensureCursorVisible()
		}
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.ensureCursorVisible()
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

	case key.Matches(msg, m.keys.Pipeline):
		return m, func() tea.Msg {
			return message.SwitchView{View: message.ViewPipelineMonitor}
		}

	case key.Matches(msg, m.keys.Back):
		return m, func() tea.Msg { return message.PopView{} }
	}

	return m, nil
}

// handleOpen opens the detail sub-view for the selected source, or toggles
// expand/collapse when the cursor is on a group header.
func (m Model) handleOpen() (Model, tea.Cmd) {
	if m.cursor < 0 || m.cursor >= len(m.flatItems) {
		return m, nil
	}

	item := m.flatItems[m.cursor]

	// Group header: toggle expand/collapse.
	if item.isHeader {
		m.groups[item.groupIdx].Collapsed = !m.groups[item.groupIdx].Collapsed
		m.rebuildFlatItems()
		return m, nil
	}

	// Source row: open detail sub-view.
	s := m.SelectedSource()
	if s == nil {
		return m, nil
	}

	m.detailOpen = true
	src := *s
	m.detailSource = &src
	return m, nil
}

// handleToggleApproach initiates the approach toggle with confirmation.
func (m Model) handleToggleApproach() (Model, tea.Cmd) {
	s := m.activeSource()
	if s == nil {
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

// handleSetOwnership activates the ownership choice prompt.
func (m Model) handleSetOwnership() (Model, tea.Cmd) {
	s := m.activeSource()
	if s == nil {
		return m, nil
	}

	m.confirmState = ConfirmSetOwnership
	m.ownershipPrompt = "Set " + s.Name + " ownership: [m]ine / [n]ot mine / [esc] cancel"
	return m, nil
}

// updateOwnershipPrompt handles the ownership choice prompt (m/n/esc).
func (m Model) updateOwnershipPrompt(msg tea.Msg) (Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("m", "M"))):
			m.confirmState = ConfirmNone
			m.ownershipPrompt = ""
			s := m.activeSource()
			if s == nil {
				return m, nil
			}
			name := s.Name
			return m, func() tea.Msg {
				return message.SetOwnershipFinished{SourceName: name, NewOwnership: "mine"}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N"))):
			m.confirmState = ConfirmNone
			m.ownershipPrompt = ""
			s := m.activeSource()
			if s == nil {
				return m, nil
			}
			name := s.Name
			return m, func() tea.Msg {
				return message.SetOwnershipFinished{SourceName: name, NewOwnership: "not_mine"}
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.confirmState = ConfirmNone
			m.ownershipPrompt = ""
			return m, nil
		}
	}
	return m, nil
}

// updateConfirm routes messages to the confirmation sub-component.
// The ConfirmResult message is handled by Update() directly.
func (m Model) updateConfirm(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmResult(result components.ConfirmResult) (Model, tea.Cmd) {
	state := m.confirmState
	m.confirmState = ConfirmNone

	if !result.Confirmed {
		return m, nil
	}

	s := m.activeSource()

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
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(msg, m.keys.Back):
			m.detailOpen = false
			m.detailSource = nil
			m.changes = nil
			return m, nil

		case key.Matches(msg, m.keys.SetOwnership):
			return m.handleSetOwnership()

		case key.Matches(msg, m.keys.ToggleApproach):
			return m.handleToggleApproach()

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
				// Update detail sub-view if open on this source.
				if m.detailOpen && m.detailSource != nil && m.detailSource.Name == msg.SourceName {
					m.detailSource.Approach = msg.NewApproach
				}
				// Persist to disk (non-fatal if it fails).
				_ = store.UpdateSource(msg.SourceName, func(s *fitness.Source) {
					s.Approach = msg.NewApproach
				})
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
				// Update detail sub-view if open on this source.
				if m.detailOpen && m.detailSource != nil && m.detailSource.Name == msg.SourceName {
					m.detailSource.Ownership = msg.NewOwnership
				}
				// Persist to disk (non-fatal if it fails).
				now := time.Now().UTC()
				_ = store.UpdateSource(msg.SourceName, func(s *fitness.Source) {
					s.Ownership = msg.NewOwnership
					s.ClassifiedAt = &now
				})
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
			return message.StatusMessage{Text: "Rollback failed: " + msg.Err.Error(), Duration: 3 * time.Second}
		}
	}
	// Remove the rolled-back change from the list.
	for i, c := range m.changes {
		if c.ID == msg.ChangeID {
			m.changes = append(m.changes[:i], m.changes[i+1:]...)
			break
		}
	}
	return m, func() tea.Msg {
		return message.StatusMessage{Text: "Rollback complete.", Duration: 3 * time.Second}
	}
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
