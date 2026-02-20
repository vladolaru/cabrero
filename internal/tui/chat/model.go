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

	vp := viewport.New(width-2, height-6)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		viewport:       vp,
		input:          ta,
		chips:          chips,
		chipsVisible:   len(chips) > 0,
		spinner:        s,
		contextPayload: contextPayload,
		width:          width,
		height:         height,
	}
}

// SetSize updates the chat panel dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.Width = width - 2
	m.viewport.Height = height - 6
	m.input.SetWidth(width - 4)
}

// Focus gives keyboard focus to the input area.
func (m *Model) Focus() {
	m.input.Focus()
}

// Blur removes keyboard focus from the input area.
func (m *Model) Blur() {
	m.input.Blur()
}

// Focused returns true if the input area has focus.
func (m Model) Focused() bool {
	return m.input.Focused()
}

// HasRevision returns true if the last assistant response contained a revision.
func (m Model) HasRevision() bool {
	return m.revision != nil
}

// Revision returns the last parsed revision, or nil.
func (m Model) Revision() *string {
	return m.revision
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
