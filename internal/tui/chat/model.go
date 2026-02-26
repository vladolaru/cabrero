// Package chat implements the AI chat panel for interrogating proposals.
package chat

import (
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// waitingMessages are shown with the spinner while waiting for the AI response.
var waitingMessages = []string{
	"Consulting the goats...",
	"Herding pirate goats...",
	"Reading the tea leaves...",
	"Asking the oracle...",
	"Thinking goat thoughts...",
	"Polishing the crystal ball...",
	"Counting citations...",
	"Interrogating the transcript...",
	"Wrangling the LLM...",
	"Pondering your question...",
}

// ChatConfig holds the configuration for a persistent chat session.
type ChatConfig struct {
	SessionID    string // persistent CC session ID (pre-blocklisted by caller)
	SystemPrompt string // rich context: proposal details, cited UUIDs, guidelines
	AllowedTools string // comma-separated path-scoped tool specs for --allowedTools
	Model        string // claude model to use (default: DefaultChatModel)
	Debug        bool
}

// ChatMessage is a single message in the chat history.
type ChatMessage struct {
	Role     string // "user" or "assistant"
	Content  string
	Rendered string // pre-rendered content (markdown for assistant, plain for user)
}

// Model is the AI chat panel model.
type Model struct {
	messages     []ChatMessage
	viewport     viewport.Model
	input        textarea.Model
	chips        []string // question chips from Haiku evaluation
	chipsVisible bool
	streaming      bool
	streamBuf      strings.Builder
	activityLines  []string // recent activity descriptions shown during streaming
	spinner        spinner.Model
	waitingMsg     string // funny message shown while waiting for AI
	config       ChatConfig
	messagesSent int // 0 = first message creates session; >0 = resume
	revision       *string
	rawContent     string // unpadded messages content for inline rendering
	width          int
	height         int
	// Focused indicates whether the chat panel has focus.
	// When false, the panel is rendered in muted colors.
	Focused bool
}

// New creates a chat model with question chips and session config.
func New(chips []string, cfg ChatConfig, width, height int) Model {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000
	ta.SetHeight(1)
	ta.SetWidth(width - 4)
	styles := ta.Styles()
	styles.Focused.Base = lipgloss.NewStyle()
	styles.Blurred.Base = lipgloss.NewStyle()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(styles)

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(shared.ColorChat)

	m := Model{
		input:        ta,
		chips:        chips,
		chipsVisible: len(chips) > 0,
		spinner:      s,
		config:       cfg,
		width:        width,
		height:       height,
	}

	vpH := m.viewportHeight()
	m.viewport = viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(vpH))
	return m
}

// SetSize updates the chat panel dimensions and re-renders content.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.viewport.SetWidth(width - 2)
	m.viewport.SetHeight(m.viewportHeight())
	m.input.SetWidth(width - 4)
	m.rerenderMessages()
	m.updateViewportContent()
}

// viewportHeight returns the viewport height after reserving space for chrome.
// Chrome: header (2) + optional chips + post-viewport newline (1) + input (1) + trailing newline (1).
// Each visible chip is 1 line ("[N] text") plus a trailing blank line after all chips.
func (m Model) viewportHeight() int {
	chrome := 5 // header (2) + post-viewport newline (1) + input (1) + trailing newline (1)
	if m.chipsVisible && len(m.chips) > 0 {
		n := len(m.chips)
		if n > 4 {
			n = 4
		}
		chrome += n + 1 // 1 line per chip + 1 trailing blank
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

// SetFocused updates the focus state and re-renders viewport content
// so that muting is applied or removed.
func (m *Model) SetFocused(focused bool) {
	if m.Focused == focused {
		return
	}
	m.Focused = focused
	m.updateViewportContent()
}

// IsInputFocused returns true when the chat text input has focus (user is typing).
func (m Model) IsInputFocused() bool {
	return m.input.Focused()
}

// HasRevision returns true if the last assistant response contained a revision.
func (m Model) HasRevision() bool {
	return m.revision != nil
}

// addMessage appends a message and updates the viewport.
// User messages use simple word wrap; assistant messages are parsed as markdown.
// Both use hanging indent so continuation lines align past the label.
func (m *Model) addMessage(role, content string) {
	w := m.width - 2
	indent := 4 // "AI: "
	if role == "user" {
		indent = 5 // "You: "
	}
	msg := ChatMessage{Role: role, Content: content}
	if role == "assistant" {
		rendered := renderMarkdown(content, w-indent)
		msg.Rendered = applyHangingIndent(rendered, indent)
	} else {
		msg.Rendered = shared.WrapHangingIndent(content, w, indent)
	}
	m.messages = append(m.messages, msg)
	m.updateViewportContent()
}

// rerenderMessages rebuilds the pre-rendered content for all messages
// at the current width. Called on resize.
func (m *Model) rerenderMessages() {
	w := m.width - 2
	for i := range m.messages {
		indent := 4 // "AI: "
		if m.messages[i].Role == "user" {
			indent = 5 // "You: "
		}
		if m.messages[i].Role == "assistant" {
			rendered := renderMarkdown(m.messages[i].Content, w-indent)
			m.messages[i].Rendered = applyHangingIndent(rendered, indent)
		} else {
			m.messages[i].Rendered = shared.WrapHangingIndent(m.messages[i].Content, w, indent)
		}
	}
}

func (m *Model) updateViewportContent() {
	if m.width == 0 {
		return
	}
	var b strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			b.WriteString(shared.ChatAccentStyle.Bold(true).Render("You:") + " " + msg.Rendered)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString(shared.ChatAccentStyle.Bold(true).Render("AI:") + " " + msg.Rendered)
			b.WriteString("\n\n")
		}
	}

	if m.streaming {
		spinnerPrefix := m.spinner.View() + " "
		b.WriteString(spinnerPrefix + shared.ChatAccentStyle.Render(m.waitingMsg))
		if len(m.activityLines) > 0 {
			// Pad activity lines to align with text after spinner.
			pad := strings.Repeat(" ", lipgloss.Width(spinnerPrefix))
			b.WriteString("\n")
			for _, line := range m.activityLines {
				b.WriteString(pad + shared.MutedStyle.Render(line) + "\n")
			}
		}
	}

	content := b.String()
	if !m.Focused {
		content = shared.MuteANSI(content)
	}
	m.rawContent = content
	wasAtBottom := m.viewport.AtBottom()
	m.viewport.SetContent(content)
	if wasAtBottom {
		m.viewport.GotoBottom()
	}
}

// applyHangingIndent adds indent spaces before all lines except the first.
func applyHangingIndent(s string, indent int) string {
	pad := strings.Repeat(" ", indent)
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
