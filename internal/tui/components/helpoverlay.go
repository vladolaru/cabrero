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

	helpDescStyle = lipgloss.NewStyle().
			Foreground(shared.ColorMuted).
			PaddingLeft(2)
)

// RenderHelpOverlay renders the help overlay with description and key binding sections.
func RenderHelpOverlay(hc shared.HelpContent, width, height int) string {
	var b strings.Builder

	b.WriteString("\n") // top padding

	// Description paragraphs.
	if hc.Description != "" {
		wrapW := width - 6 // padding + margin
		if wrapW < 40 {
			wrapW = 40
		}
		wrapStyle := helpDescStyle.Width(wrapW)
		for _, para := range strings.Split(hc.Description, "\n\n") {
			b.WriteString(wrapStyle.Render(strings.TrimSpace(para)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

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
