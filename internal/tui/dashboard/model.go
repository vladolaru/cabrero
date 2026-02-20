// Package dashboard implements the dashboard view — the TUI home screen.
package dashboard

import (
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui"
	"github.com/vladolaru/cabrero/internal/tui/message"
)

// Sort orders for the proposal list.
const (
	SortNewest     = "newest"
	SortOldest     = "oldest"
	SortConfidence = "confidence"
	SortType       = "type"
)

var sortOrders = []string{SortNewest, SortOldest, SortConfidence, SortType}

// Model is the dashboard view model.
type Model struct {
	proposals    []pipeline.ProposalWithSession
	filtered     []pipeline.ProposalWithSession // after filter applied
	cursor       int
	stats        message.DashboardStats
	filterInput  textinput.Model
	filterActive bool
	filterText   string
	sortOrder    string
	width        int
	height       int
	keys         *tui.KeyMap
	config       *tui.Config
}

// New creates a dashboard model with loaded data.
func New(proposals []pipeline.ProposalWithSession, stats message.DashboardStats, keys *tui.KeyMap, cfg *tui.Config) Model {
	fi := textinput.New()
	fi.Placeholder = "type:skill target:docx or free text..."
	fi.Prompt = "/ "

	sortOrder := cfg.Dashboard.SortOrder
	if sortOrder == "" {
		sortOrder = SortNewest
	}

	m := Model{
		proposals:   proposals,
		cursor:      0,
		stats:       stats,
		filterInput: fi,
		sortOrder:   sortOrder,
		keys:        keys,
		config:      cfg,
	}
	m.applyFilter()
	return m
}

// SelectedProposal returns the proposal at the current cursor position.
func (m Model) SelectedProposal() *pipeline.ProposalWithSession {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	p := m.filtered[m.cursor]
	return &p
}

// CycleSortOrder advances to the next sort order.
func (m *Model) CycleSortOrder() {
	for i, s := range sortOrders {
		if s == m.sortOrder {
			m.sortOrder = sortOrders[(i+1)%len(sortOrders)]
			m.applyFilter()
			return
		}
	}
	m.sortOrder = SortNewest
	m.applyFilter()
}

func (m *Model) applyFilter() {
	m.filtered = m.proposals
	// Future: implement actual filtering by type/target/text.
	// For now, show all proposals.
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}
