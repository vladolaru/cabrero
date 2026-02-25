package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
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

	// Drop trailing bindings until the bar fits in one line.
	sep := "  "
	contentWidth := width - 2 // statusBarStyle has Padding(0, 1)
	for len(parts) > 0 {
		bar := strings.Join(parts, sep)
		if lipgloss.Width(bar) <= contentWidth {
			return statusBarStyle.Width(width).Render(bar)
		}
		parts = parts[:len(parts)-1]
	}

	return statusBarStyle.Width(width).Render("")
}
