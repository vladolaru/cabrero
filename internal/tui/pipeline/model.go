// Package pipeline implements the pipeline monitor view for the review TUI.
// It displays daemon status, pipeline activity statistics, recent pipeline runs
// with inline expansion, and prompt version tracking.
package pipeline

import (
	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

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
}

// Refresh updates the pipeline data while preserving cursor and expansion state.
func (m *Model) Refresh(runs []pl.PipelineRun, stats pl.PipelineStats, prompts []pl.PromptVersion, dashStats message.DashboardStats) {
	m.runs = runs
	m.stats = stats
	m.prompts = prompts
	m.dashStats = dashStats
	m.statusMsg = ""
	// Clamp cursor to the new data bounds.
	if m.cursor >= len(m.runs) {
		m.cursor = max(0, len(m.runs)-1)
	}
	if m.expandedIdx >= len(m.runs) {
		m.expandedIdx = -1
	}
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
