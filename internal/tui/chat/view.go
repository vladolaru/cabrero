package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	chatAccent     = lipgloss.NewStyle().Foreground(shared.ColorChat)
	chatLabelStyle = lipgloss.NewStyle().Foreground(shared.ColorChat).Bold(true)
	chatMuted      = lipgloss.NewStyle().Foreground(shared.ColorMuted)
	chipNumStyle   = lipgloss.NewStyle().Foreground(shared.ColorChat).Bold(true)
	chipTextStyle  = lipgloss.NewStyle().Foreground(shared.ColorChat)
	chipMutedNum   = lipgloss.NewStyle().Foreground(shared.ColorMuted).Bold(true)
	chipMutedText  = lipgloss.NewStyle().Foreground(shared.ColorMuted)
)

// renderChip formats a single question chip as "  [N] text".
func renderChip(idx int, text string, focused bool) string {
	numStyle := chipMutedNum
	textStyle := chipMutedText
	if focused {
		numStyle = chipNumStyle
		textStyle = chipTextStyle
	}
	return numStyle.Render(fmt.Sprintf("[%d]", idx+1)) + " " + textStyle.Render(text)
}

// RenderInline returns the chat content with a bounded viewport for messages.
// Used in narrow mode where the chat is part of the detail's scrollable viewport.
func (m Model) RenderInline() string {
	headerStyle := chatAccent
	if !m.Focused {
		headerStyle = chatMuted
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("  ASK ME ABOUT THIS PROPOSAL"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("─", 17))
	b.WriteString("\n")

	// Question chips.
	if m.chipsVisible && len(m.chips) > 0 {
		for i, chip := range m.chips {
			if i >= 4 {
				break
			}
			b.WriteString(renderChip(i, chip, m.Focused))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Chat messages viewport (bounded height, indented to match chrome).
	b.WriteString(shared.IndentBlock(m.viewport.View(), 2))
	b.WriteString("\n")

	// Fill remaining space to push input to the bottom of the chat area.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 1 // -1 for input line
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Input area.
	if m.input.Focused() {
		content += "  " + m.input.View()
	} else {
		content += chatMuted.Render("  Press enter to type...")
	}
	content += "\n"

	return content
}

// View renders the chat panel (wide mode, horizontal split).
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	headerStyle := chatAccent
	if !m.Focused {
		headerStyle = chatMuted
	}

	var b strings.Builder

	b.WriteString(headerStyle.Render("Ask me about this proposal"))
	b.WriteString("\n\n")

	// Question chips.
	if m.chipsVisible && len(m.chips) > 0 {
		for i, chip := range m.chips {
			if i >= 4 {
				break
			}
			b.WriteString(renderChip(i, chip, m.Focused))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Chat messages viewport.
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Fill remaining space to push input to the bottom.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 3 // blank line + input line + trailing newline
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Input area with a blank line above for breathing room.
	content += "\n"
	if m.input.Focused() {
		content += m.input.View()
	} else {
		content += chatMuted.Render("Press enter to type...")
	}
	content += "\n"

	return content
}
