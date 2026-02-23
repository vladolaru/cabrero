package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	helpViewTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(shared.ColorFgBold)

	helpViewDescStyle = lipgloss.NewStyle().
				Foreground(shared.ColorMuted)

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

// RenderHelpOverlay renders a context-aware help overlay with title, description, and key binding sections.
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
	var b strings.Builder

	b.WriteString("\n") // top padding

	// View title.
	b.WriteString("  ")
	b.WriteString(helpViewTitleStyle.Render(hc.Title))
	b.WriteString("\n")

	// View description.
	if hc.Description != "" {
		b.WriteString("  ")
		b.WriteString(helpViewDescStyle.Render(hc.Description))
		b.WriteString("\n")
	}

	b.WriteString("\n") // gap before sections

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
