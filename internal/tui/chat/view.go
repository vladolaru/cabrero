package chat

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	chatAccent = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#00695C", Dark: "#4DB6AC"})
	chatMuted = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"})
	chipStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#00695C", Dark: "#4DB6AC"}).
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

	// Input area.
	if m.input.Focused() {
		b.WriteString(m.input.View())
	} else {
		b.WriteString(chatMuted.Render("  Press enter to type..."))
	}

	return b.String()
}
