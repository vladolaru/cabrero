package components

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

var (
	statusBarStyle = lipgloss.NewStyle().
		Padding(0, 1)

	statusKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Padding(0, 1)

	statusDescStyle = lipgloss.NewStyle().
		Padding(0, 1, 0, 0)
)

// RenderStatusBar renders a bottom status bar with context-sensitive shortcuts.
// If timedMsg is non-empty, it overlays the shortcuts.
func RenderStatusBar(bindings []key.Binding, timedMsg string, width int) string {
	if timedMsg != "" {
		return statusBarStyle.Width(width).Render(timedMsg)
	}

	var parts []string
	for _, b := range bindings {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		part := statusKeyStyle.Render(h.Key) + statusDescStyle.Render(h.Desc)
		parts = append(parts, part)
	}

	bar := strings.Join(parts, "  ")
	return statusBarStyle.Width(width).Render(bar)
}
