// Package dashboard implements the dashboard view — the TUI home screen.
package dashboard

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Sort orders for the item list.
const (
	SortNewest     = "newest"
	SortOldest     = "oldest"
	SortConfidence = "confidence"
	SortType       = "type"
)

var sortOrders = []string{SortNewest, SortOldest, SortConfidence, SortType}

// DashboardItem wraps either a proposal or a fitness report for the unified list.
type DashboardItem struct {
	Proposal      *pipeline.ProposalWithSession
	FitnessReport *fitness.Report
}

// IsProposal returns true if this item wraps a proposal.
func (d DashboardItem) IsProposal() bool {
	return d.Proposal != nil
}

// IsFitnessReport returns true if this item wraps a fitness report.
func (d DashboardItem) IsFitnessReport() bool {
	return d.FitnessReport != nil
}

// TypeIndicator returns "●" for proposals or "◎" for fitness reports.
func (d DashboardItem) TypeIndicator() string {
	if d.IsProposal() {
		return indicatorProposal
	}
	return indicatorFitness
}

// TypeName returns a human-readable type name for the item.
func (d DashboardItem) TypeName() string {
	if d.IsProposal() {
		return d.Proposal.Proposal.Type
	}
	return "fitness_report"
}

// Target returns the target name for the item.
func (d DashboardItem) Target() string {
	if d.IsProposal() {
		return d.Proposal.Proposal.Target
	}
	return d.FitnessReport.SourceName
}

// Confidence returns the confidence level for proposals, or a health summary for reports.
func (d DashboardItem) Confidence() string {
	if d.IsProposal() {
		return d.Proposal.Proposal.Confidence
	}
	// For fitness reports, show the followed percentage as a health indicator.
	return fmt.Sprintf("%d%% health", int(d.FitnessReport.Assessment.Followed.Percent))
}

// Model is the dashboard view model.
type Model struct {
	items        []DashboardItem
	filtered     []DashboardItem // after filter applied
	cursor       int
	stats        message.DashboardStats
	viewport     viewport.Model
	filterInput  textinput.Model
	filterActive bool
	filterText   string
	statusMsg    string
	statusExpiry time.Time
	sortOrder    string
	width        int
	height       int
	keys         *shared.KeyMap
	config       *shared.Config
}

// New creates a dashboard model with loaded data.
func New(proposals []pipeline.ProposalWithSession, reports []fitness.Report,
	stats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model {

	fi := textinput.New()
	fi.Placeholder = "type:skill target:docx or free text..."
	fi.Prompt = "/ "

	sortOrder := cfg.Dashboard.SortOrder
	if sortOrder == "" {
		sortOrder = SortNewest
	}

	// Build the unified item list: proposals first, then fitness reports.
	items := make([]DashboardItem, 0, len(proposals)+len(reports))
	for i := range proposals {
		items = append(items, DashboardItem{Proposal: &proposals[i]})
	}
	for i := range reports {
		items = append(items, DashboardItem{FitnessReport: &reports[i]})
	}

	m := Model{
		items:       items,
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

// SelectedItem returns the DashboardItem at the current cursor position.
func (m Model) SelectedItem() *DashboardItem {
	if len(m.filtered) == 0 || m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	item := m.filtered[m.cursor]
	return &item
}

// SelectedProposal returns the proposal at the current cursor position, or nil
// if the cursor is on a fitness report or the list is empty.
func (m Model) SelectedProposal() *pipeline.ProposalWithSession {
	item := m.SelectedItem()
	if item == nil || !item.IsProposal() {
		return nil
	}
	return item.Proposal
}

// SelectedFitnessReport returns the fitness report at the current cursor position,
// or nil if the cursor is on a proposal or the list is empty.
func (m Model) SelectedFitnessReport() *fitness.Report {
	item := m.SelectedItem()
	if item == nil || !item.IsFitnessReport() {
		return nil
	}
	return item.FitnessReport
}

// HasActiveInput returns true when the filter input is active (user is typing).
func (m Model) HasActiveInput() bool {
	return m.filterActive
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
	m.filtered = m.items
	// Future: implement actual filtering by type/target/text.
	// For now, show all items.
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
	m.updateContent()
}

// viewportHeight returns the height available for the scrollable item list.
// Fixed chrome: status/filter bar (1). When items exist: column header (1) + sort line (1).
func (m Model) viewportHeight() int {
	chrome := 1 // status/filter bar
	if len(m.filtered) > 0 {
		chrome += 2 // column header + sort indicator
	}
	h := m.height - chrome
	if h < 1 {
		h = 1
	}
	return h
}

// updateContent rebuilds the viewport content from the current items and cursor.
// Only item rows go in the viewport; column headers and sort indicator are rendered
// as fixed chrome in View().
func (m *Model) updateContent() {
	if m.width == 0 || m.height == 0 {
		return
	}

	m.viewport.SetHeight(m.viewportHeight())

	if len(m.filtered) == 0 {
		m.viewport.SetContent("\n" + shared.MutedStyle.Render("  "+components.EmptyProposals()) + "\n")
	} else {
		m.viewport.SetContent(m.renderItemRows())
	}

	m.ensureCursorVisible()
}

// ensureCursorVisible adjusts the viewport scroll offset so the cursor row is visible.
func (m *Model) ensureCursorVisible() {
	if len(m.filtered) == 0 {
		return
	}
	cursorLine := m.cursor
	yOff := m.viewport.YOffset()
	h := m.viewport.Height()

	if cursorLine < yOff {
		m.viewport.SetYOffset(cursorLine)
	} else if cursorLine >= yOff+h {
		m.viewport.SetYOffset(cursorLine - h + 1)
	}
}
