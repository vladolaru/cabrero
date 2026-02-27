// Package ops implements the Operations view for reliability and runtime signals.
package ops

import (
	"charm.land/bubbles/v2/viewport"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the Operations view model.
type Model struct {
	// Domain data.
	stats pipeline.OpsStats

	// Navigation.
	cursor int // cursor over recent events list

	// Layout.
	width  int
	height int

	// Scrollable body.
	viewport viewport.Model

	// Shared deps.
	keys   *shared.KeyMap
	config *shared.Config

	// Status bar.
	statusMsg string
}

// New creates a new Operations view model.
func New(stats pipeline.OpsStats, keys *shared.KeyMap, cfg *shared.Config) Model {
	return Model{
		stats:  stats,
		keys:   keys,
		config: cfg,
	}
}

// SetSize updates the layout dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	vpH := height - 1 // 1 line for status bar
	if vpH < 1 {
		vpH = 1
	}
	m.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(vpH))
	m.refreshViewport()
}

// UpdateStats replaces the stats data and refreshes the viewport.
func (m *Model) UpdateStats(stats pipeline.OpsStats) {
	m.stats = stats
	m.refreshViewport()
}

// HasActivePrompt returns false — the ops view has no confirmation prompts.
func (m Model) HasActivePrompt() bool {
	return false
}

func (m *Model) refreshViewport() {
	if m.width == 0 {
		return
	}
	m.viewport.SetContent(m.renderBody())
}
