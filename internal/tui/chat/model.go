// Package chat implements the AI chat panel for interrogating proposals.
package chat

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
)

// ChatMessage is a single message in the chat history.
type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

// Model is the AI chat panel model.
type Model struct {
	messages       []ChatMessage
	viewport       viewport.Model
	input          textarea.Model
	chips          []string // question chips from Haiku evaluation
	chipsVisible   bool
	streaming      bool
	streamBuf      strings.Builder
	spinner        spinner.Model
	contextPayload string // citation chain JSON for system context
	revision       *string
	width          int
	height         int
}

// New creates a chat model with question chips and context payload.
func New(chips []string, contextPayload string, width, height int) Model {
	ta := textarea.New()
	ta.Placeholder = "Type a question..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000
	ta.SetHeight(1)
	ta.SetWidth(width - 4)

	s := spinner.New()
	s.Spinner = spinner.Dot

	m := Model{
		input:          ta,
		chips:          chips,
		chipsVisible:   len(chips) > 0,
		spinner:        s,
		contextPayload: contextPayload,
		width:          width,
		height:         height,
	}

	vpH := m.viewportHeight()
	m.viewport = viewport.New(width-2, vpH)
	return m
}

// SetSize updates the chat panel dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width - 2
	m.viewport.Height = m.viewportHeight()
	m.input.SetWidth(width - 4)
}

// viewportHeight returns the viewport height after reserving space for chrome.
// Chrome: header (2 lines) + optional chips + newline after viewport (1) + input (1).
// Each visible chip is 3 visual lines (border top + content + border bottom) plus
// a trailing newline, and a blank line follows the last chip.
func (m Model) viewportHeight() int {
	chrome := 4 // header (2) + post-viewport newline (1) + input (1)
	if m.chipsVisible && len(m.chips) > 0 {
		n := len(m.chips)
		if n > 4 {
			n = 4
		}
		chrome += n*3 + 1 // 3 lines per chip (border) + 1 trailing blank
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

// HasRevision returns true if the last assistant response contained a revision.
func (m Model) HasRevision() bool {
	return m.revision != nil
}

// addMessage appends a message and updates the viewport.
func (m *Model) addMessage(role, content string) {
	m.messages = append(m.messages, ChatMessage{Role: role, Content: content})
	m.updateViewportContent()
}

func (m *Model) updateViewportContent() {
	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			b.WriteString("You: ")
			b.WriteString(msg.Content)
		case "assistant":
			b.WriteString("AI: ")
			b.WriteString(msg.Content)
		}
		b.WriteString("\n\n")
	}

	if m.streaming {
		b.WriteString("AI: ")
		b.WriteString(m.streamBuf.String())
		b.WriteString("▊") // blinking cursor indicator
	}

	m.viewport.SetContent(b.String())
	m.viewport.GotoBottom()
}
