package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	chatAccent = lipgloss.NewStyle().Foreground(shared.ColorChat)
	chatMuted  = lipgloss.NewStyle().Foreground(shared.ColorMuted)
	chipStyle  = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(shared.ColorChat).
			Padding(0, 1)
)

// View renders the chat panel.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	b.WriteString(chatAccent.Render("  Ask me about this proposal"))
	b.WriteString("\n\n")

	// Question chips.
	if m.chipsVisible && len(m.chips) > 0 {
		for i, chip := range m.chips {
			if i >= 4 {
				break
			}
			label := fmt.Sprintf(" %d  %s", i+1, chip)
			b.WriteString("  " + chipStyle.Render(label))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Chat messages viewport.
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Fill remaining space to match parent height.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 1 // -1 for input line
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Input area.
	if m.input.Focused() {
		content += m.input.View()
	} else {
		content += chatMuted.Render("  Press enter to type...")
	}

	return content
}
