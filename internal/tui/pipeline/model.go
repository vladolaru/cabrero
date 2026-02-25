// Package pipeline implements the pipeline monitor view for the TUI.
// It displays daemon status, pipeline activity statistics, recent pipeline runs
// with inline expansion, and prompt version tracking.
package pipeline

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// statusClearMsg is sent after a delay to clear the status bar message.
type statusClearMsg struct{}

// Model is the pipeline monitor view model.
type Model struct {
	runs        []pl.PipelineRun
	stats       pl.PipelineStats
	prompts     []pl.PromptVersion
	dashStats   message.DashboardStats
	cursor      int
	expandedIdx int // -1 means no run expanded
	confirm     components.ConfirmModel
	retrying    string // session ID being retried, "" if none
	statusMsg   string // timed status bar message (e.g. "Refreshing…")
	width       int
	height      int
	viewport    viewport.Model
	keys        *shared.KeyMap
	config      *shared.Config
}

// New creates a pipeline monitor model with loaded data.
func New(runs []pl.PipelineRun, stats pl.PipelineStats, prompts []pl.PromptVersion, dashStats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model {
	return Model{
		runs:        runs,
		stats:       stats,
		prompts:     prompts,
		dashStats:   dashStats,
		expandedIdx: -1,
		keys:        keys,
		config:      cfg,
	}
}

// SetSize updates the viewport dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	vpH := height - 1 // reserve 1 line for status bar
	if vpH < 1 {
		vpH = 1
	}
	m.viewport = viewport.New(viewport.WithWidth(width), viewport.WithHeight(vpH))
	m.refreshViewport()
}

// refreshViewport rebuilds the viewport content from all pipeline sections.
// Call after data refresh, resize, or cursor/expand state changes.
func (m *Model) refreshViewport() {
	if m.width == 0 {
		return
	}
	// Don't render into viewport when confirm is active (View() returns early).
	if m.confirm.Active {
		return
	}
	var sections []string
	sections = append(sections, m.renderDaemonHeader())
	sections = append(sections, m.renderActivityStats())
	sections = append(sections, m.renderRecentRuns())
	if m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderModels())
	}
	if len(m.prompts) > 0 && m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderPrompts())
	}
	m.viewport.SetContent(strings.Join(sections, "\n\n"))
}

// Refresh updates the pipeline data while preserving cursor and expansion state.
// Returns a tea.Cmd when a timed status clear is needed (manual refresh).
func (m *Model) Refresh(runs []pl.PipelineRun, stats pl.PipelineStats, prompts []pl.PromptVersion, dashStats message.DashboardStats) tea.Cmd {
	m.runs = runs
	m.stats = stats
	m.prompts = prompts
	m.dashStats = dashStats
	// Clamp cursor to the new data bounds.
	if m.cursor >= len(m.runs) {
		m.cursor = max(0, len(m.runs)-1)
	}
	if m.expandedIdx >= len(m.runs) {
		m.expandedIdx = -1
	}
	m.refreshViewport()
	// Show confirmation only after a manual refresh.
	if m.statusMsg != "" {
		m.statusMsg = "Data refreshed."
		return tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg {
			return statusClearMsg{}
		})
	}
	return nil
}

// HasActivePrompt returns true when a confirmation prompt is active
// and the view should handle Esc itself (to dismiss the prompt).
func (m Model) HasActivePrompt() bool {
	return m.confirm.Active
}

// SelectedRun returns the run at the current cursor position, or nil.
func (m Model) SelectedRun() *pl.PipelineRun {
	if m.cursor < 0 || m.cursor >= len(m.runs) {
		return nil
	}
	return &m.runs[m.cursor]
}
