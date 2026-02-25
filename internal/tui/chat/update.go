package chat

import (
	"math/rand"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

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
		if msg.newTurn {
			// New assistant turn — discard intermediate text from earlier turns.
			m.streamBuf.Reset()
		}
		if msg.token != "" {
			m.streamBuf.WriteString(msg.token)
		}
		if msg.activity != "" {
			m.activityLines = append(m.activityLines, msg.activity)
			if len(m.activityLines) > 3 {
				m.activityLines = m.activityLines[len(m.activityLines)-3:]
			}
		}
		m.updateViewportContent()
		return m, msg.NextCmd()

	case message.ChatStreamDone:
		m.streaming = false
		m.activityLines = nil
		response := strings.TrimLeft(m.streamBuf.String(), " \t\n\r")
		m.streamBuf.Reset()
		m.addMessage("assistant", response)

		// Parse for revision blocks.
		rev := parseRevision(response)
		m.revision = rev

		return m, nil

	case message.ChatStreamError:
		m.streaming = false
		m.activityLines = nil
		m.streamBuf.Reset()
		m.addMessage("assistant", "Error: "+msg.Err.Error())
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	// Forward spinner ticks when streaming.
	if m.streaming {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
			m.updateViewportContent()
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

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
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

	// Enter focuses the input when not already focused.
	if key.Matches(msg, key.NewBinding(key.WithKeys("enter"))) {
		m.input.Focus()
		return m, nil
	}

	// Forward scroll keys to viewport.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
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
	m.waitingMsg = waitingMessages[rand.Intn(len(waitingMessages))]
	m.streamBuf.Reset()
	firstMessage := m.messagesSent == 0
	m.messagesSent++
	m.updateViewportContent()
	return m, tea.Batch(m.spinner.Tick, StartChat(text, m.config, firstMessage))
}
