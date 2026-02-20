package chat

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Update handles messages for the chat panel.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
		return m, nil

	case chatTokenWithContinuation:
		m.streamBuf.WriteString(msg.token)
		m.updateViewportContent()
		return m, msg.NextCmd()

	case message.ChatStreamDone:
		m.streaming = false
		response := m.streamBuf.String()
		m.streamBuf.Reset()
		m.addMessage("assistant", response)

		// Parse for revision blocks.
		rev := parseRevision(response)
		m.revision = rev

		return m, nil

	case message.ChatStreamError:
		m.streaming = false
		m.streamBuf.Reset()
		m.addMessage("assistant", "Error: "+msg.Err.Error())
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Forward spinner ticks when streaming.
	if m.streaming {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Forward to viewport when not typing.
	if !m.input.Focused() {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Chip keys — only when chips are visible and input is not focused.
	if m.chipsVisible && !m.input.Focused() {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
			return m.sendChip(0)
		case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
			return m.sendChip(1)
		case key.Matches(msg, key.NewBinding(key.WithKeys("3"))):
			return m.sendChip(2)
		case key.Matches(msg, key.NewBinding(key.WithKeys("4"))):
			return m.sendChip(3)
		}
	}

	// When input is focused, handle typing.
	if m.input.Focused() {
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			text := m.input.Value()
			if text == "" {
				return m, nil
			}
			m.input.Reset()
			m.chipsVisible = false // hide chips after manual input
			return m.sendMessage(text)
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.input.Blur()
			return m, nil
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) sendChip(idx int) (Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.chips) {
		return m, nil
	}
	return m.sendMessage(m.chips[idx])
}

func (m Model) sendMessage(text string) (Model, tea.Cmd) {
	m.addMessage("user", text)
	m.streaming = true
	m.streamBuf.Reset()
	m.updateViewportContent()
	return m, StartChat(text, m.contextPayload)
}
