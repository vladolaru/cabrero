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
	// Pre-compute each part's rendered width once to avoid O(N²) re-joins.
	sep := "  "
	sepWidth := lipgloss.Width(sep)
	contentWidth := width - 2 // statusBarStyle has Padding(0, 1)

	total := 0
	cutoff := 0
	for i, p := range parts {
		w := lipgloss.Width(p)
		if i > 0 {
			w += sepWidth
		}
		if total+w > contentWidth {
			break
		}
		total += w
		cutoff = i + 1
	}

	if cutoff > 0 {
		return statusBarStyle.Width(width).Render(strings.Join(parts[:cutoff], sep))
	}
	return statusBarStyle.Width(width).Render("")
}
