package logview

import (
	"fmt"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// SubHeader returns the view title and stats for the log viewer.
func (m Model) SubHeader() string {
	var followIndicator string
	if m.followMode {
		followIndicator = shared.SuccessStyle.Render("●")
	} else {
		followIndicator = shared.MutedStyle.Render("○")
	}
	statsLine := fmt.Sprintf("  %d entries  ·  follow %s", len(m.entries), followIndicator)
	return shared.RenderSubHeader("  Log Viewer", statsLine)
}

// View renders the log viewer.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Viewport content.
	content := m.viewport.View()

	// Bottom bar: search input or status bar.
	var bottom string
	if m.searchActive {
		bottom = "/ " + m.searchInput.View()
	} else {
		timedMsg := m.statusMsg
		if m.searchTerm != "" && len(m.matches) > 0 {
			timedMsg = fmt.Sprintf("[%d/%d matches]", m.matchIdx+1, len(m.matches))
		}
		bottom = components.RenderStatusBar(m.keys.LogViewShortHelp(), timedMsg, m.width)
	}

	return content + "\n" + bottom
}
