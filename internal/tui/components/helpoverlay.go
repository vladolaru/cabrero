package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	helpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(shared.ColorMuted)

	helpKeyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(shared.ColorAccent).
			Width(14).
			Align(lipgloss.Right)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(shared.ColorFgBold).
			PaddingLeft(2)
)

// RenderHelpOverlay renders a context-aware help overlay from the given sections.
func RenderHelpOverlay(sections []shared.HelpSection, width, height int) string {
	var b strings.Builder

	b.WriteString("\n") // top padding

	for i, section := range sections {
		if i > 0 {
			b.WriteString("\n")
		}
		// Section title.
		b.WriteString("  ")
		b.WriteString(helpTitleStyle.Render(section.Title))
		b.WriteString("\n")

		// Entries.
		for _, entry := range section.Entries {
			line := "  " + helpKeyStyle.Render(entry.Key) + helpDescStyle.Render(entry.Desc)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}
