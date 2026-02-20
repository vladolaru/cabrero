package logview

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorFgBold)
	followOnStyle  = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	followOffStyle = lipgloss.NewStyle().Foreground(shared.ColorMuted)
)

// View renders the log viewer.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Title with follow indicator.
	var followIndicator string
	if m.followMode {
		followIndicator = followOnStyle.Render("●")
	} else {
		followIndicator = followOffStyle.Render("○")
	}
	title := titleStyle.Render("Log Viewer") + "  follow " + followIndicator

	// Viewport content.
	content := m.viewport.View()

	// Bottom bar: search input or status bar.
	var bottom string
	if m.searchActive {
		bottom = "/ " + m.searchInput.View()
	} else {
		// 3-arg call: bindings, timedMsg, width.
		bottom = components.RenderStatusBar(m.keys.LogViewShortHelp(), "", m.width)
		if m.searchTerm != "" && len(m.matches) > 0 {
			bottom = fmt.Sprintf("[%d/%d matches] %s", m.matchIdx+1, len(m.matches), bottom)
		}
	}

	return title + "\n" + content + "\n" + bottom
}
