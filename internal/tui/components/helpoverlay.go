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

	helpEntryDescStyle = lipgloss.NewStyle().
				Foreground(shared.ColorFgBold).
				PaddingLeft(2)
)

// RenderHelpOverlay renders key binding sections for the help overlay.
// The view title and description are provided by the sub-header, so this
// function renders only the key binding sections.
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
	var b strings.Builder

	b.WriteString("\n") // top padding

	for i, section := range hc.Sections {
		if i > 0 {
			b.WriteString("\n")
		}
		// Section title.
		b.WriteString("  ")
		b.WriteString(helpTitleStyle.Render(section.Title))
		b.WriteString("\n")

		// Entries.
		for _, entry := range section.Entries {
			line := "  " + helpKeyStyle.Render(entry.Key) + helpEntryDescStyle.Render(entry.Desc)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}
