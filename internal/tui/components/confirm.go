// Package components provides shared TUI components used across views.
package components

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
)

// ConfirmResult is sent when the user completes a confirmation prompt.
type ConfirmResult struct {
	Confirmed bool
}

// RevisionChoice is sent for the extended approve-with-revision prompt.
type RevisionChoice struct {
	Choice string // "original", "revision", or "cancel"
}

// ConfirmModel is an inline [y/N] confirmation component.
type ConfirmModel struct {
	Prompt string
	Active bool
}

// NewConfirm creates a confirmation prompt.
func NewConfirm(prompt string) ConfirmModel {
	return ConfirmModel{Prompt: prompt, Active: true}
}

// Update handles key input for the confirm prompt.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))):
			m.Active = false
			return m, func() tea.Msg { return ConfirmResult{Confirmed: true} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("n", "N", "esc"))):
			m.Active = false
			return m, func() tea.Msg { return ConfirmResult{Confirmed: false} }
		}
	}

	return m, nil
}

// View renders the confirmation prompt.
func (m ConfirmModel) View() string {
	if !m.Active {
		return ""
	}
	return m.Prompt + " [y/N] "
}

// RevisionConfirmModel is an extended confirmation for approve-with-revision.
// Prompts: [o]riginal / [r]evision / [c]ancel
type RevisionConfirmModel struct {
	Prompt string
	Active bool
}

// NewRevisionConfirm creates the extended approve prompt.
func NewRevisionConfirm(prompt string) RevisionConfirmModel {
	return RevisionConfirmModel{Prompt: prompt, Active: true}
}

// Update handles key input for the revision choice prompt.
func (m RevisionConfirmModel) Update(msg tea.Msg) (RevisionConfirmModel, tea.Cmd) {
	if !m.Active {
		return m, nil
	}

	if msg, ok := msg.(tea.KeyMsg); ok {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("o", "O"))):
			m.Active = false
			return m, func() tea.Msg { return RevisionChoice{Choice: "original"} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("r", "R"))):
			m.Active = false
			return m, func() tea.Msg { return RevisionChoice{Choice: "revision"} }
		case key.Matches(msg, key.NewBinding(key.WithKeys("c", "C", "esc"))):
			m.Active = false
			return m, func() tea.Msg { return RevisionChoice{Choice: "cancel"} }
		}
	}

	return m, nil
}

// View renders the revision choice prompt.
func (m RevisionConfirmModel) View() string {
	if !m.Active {
		return ""
	}
	return m.Prompt + " [o]riginal / [r]evision / [c]ancel "
}
