// Package dashboard implements the dashboard view — the TUI home screen.
package dashboard

import (
	"fmt"
	"slices"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/pipeline"
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

// FilterValue implements list.Item. Returns a tagged string used by dashboardFilter.
// Format: "type:<TypeName> target:<Target> confidence:<Confidence>"
func (d DashboardItem) FilterValue() string {
	return "type:" + d.TypeName() + " target:" + d.Target() + " confidence:" + d.Confidence()
}

// Model is the dashboard view model.
type Model struct {
	items        []DashboardItem // source of truth for sorting; also passed to list
	list         list.Model
	stats        message.DashboardStats
	statusMsg    string
	statusExpiry time.Time
	sortOrder    string
	keys         *shared.KeyMap
	config       *shared.Config
	reports      []fitness.Report // kept for Reload after proposal-only refreshes
}

// New creates a dashboard model with loaded data.
func New(proposals []pipeline.ProposalWithSession, reports []fitness.Report,
	stats message.DashboardStats, keys *shared.KeyMap, cfg *shared.Config) Model {

	sortOrder := cfg.Dashboard.SortOrder
	if sortOrder == "" {
		sortOrder = SortNewest
	}

	// Build and sort the unified item list.
	items := buildItems(proposals, reports)
	sorted := sortItems(items, sortOrder)

	l := newList(sorted, keys)

	return Model{
		items:     items,
		list:      l,
		stats:     stats,
		sortOrder: sortOrder,
		keys:      keys,
		config:    cfg,
		reports:   reports,
	}
}

// buildItems constructs the unified item slice: proposals first, then fitness reports.
func buildItems(proposals []pipeline.ProposalWithSession, reports []fitness.Report) []DashboardItem {
	items := make([]DashboardItem, 0, len(proposals)+len(reports))
	for i := range proposals {
		items = append(items, DashboardItem{Proposal: &proposals[i]})
	}
	for i := range reports {
		items = append(items, DashboardItem{FitnessReport: &reports[i]})
	}
	return items
}

// sortItems returns a new slice sorted by the given sort order.
func sortItems(items []DashboardItem, order string) []DashboardItem {
	out := slices.Clone(items)
	switch order {
	case SortOldest:
		slices.Reverse(out)
	case SortConfidence:
		slices.SortStableFunc(out, func(a, b DashboardItem) int {
			return confidenceRank(b) - confidenceRank(a) // higher confidence first
		})
	case SortType:
		slices.SortStableFunc(out, func(a, b DashboardItem) int {
			if a.TypeName() < b.TypeName() {
				return -1
			}
			if a.TypeName() > b.TypeName() {
				return 1
			}
			return 0
		})
	default: // SortNewest — keep original order
	}
	return out
}

// confidenceRank maps confidence strings to sortable integers.
func confidenceRank(d DashboardItem) int {
	if d.IsFitnessReport() {
		return int(d.FitnessReport.Assessment.Followed.Percent) // 0–100
	}
	switch d.Proposal.Proposal.Confidence {
	case "high":
		return 300
	case "medium":
		return 200
	case "low":
		return 100
	default:
		return 0
	}
}

// newList constructs a configured list.Model for the dashboard.
func newList(items []DashboardItem, keys *shared.KeyMap) list.Model {
	listItems := toListItems(items)
	l := list.New(listItems, dashboardDelegate{}, 0, 0)

	// Disable all list chrome — we render our own.
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetShowFilter(false) // we render filter input ourselves in the status bar area
	l.DisableQuitKeybindings()

	// Custom filter for type:/target: prefix syntax.
	l.Filter = dashboardFilter

	// Remap list navigation to our KeyMap (respects arrows vs vim setting).
	l.KeyMap.CursorUp = keys.Up
	l.KeyMap.CursorDown = keys.Down
	l.KeyMap.NextPage = keys.HalfPageDown
	l.KeyMap.PrevPage = keys.HalfPageUp
	l.KeyMap.GoToStart = keys.GotoTop
	l.KeyMap.GoToEnd = keys.GotoBottom
	l.KeyMap.Filter = keys.Filter
	l.KeyMap.ClearFilter = keys.Back
	l.KeyMap.CancelWhileFiltering = keys.Back
	l.KeyMap.AcceptWhileFiltering = keys.Open

	// Disable list's own quit/help bindings (root handles these).
	off := key.NewBinding(key.WithDisabled())
	l.KeyMap.Quit = off
	l.KeyMap.ForceQuit = off
	l.KeyMap.ShowFullHelp = off
	l.KeyMap.CloseFullHelp = off

	return l
}

func toListItems(items []DashboardItem) []list.Item {
	out := make([]list.Item, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// SelectedItem returns the DashboardItem at the current cursor position.
func (m Model) SelectedItem() *DashboardItem {
	item := m.list.SelectedItem()
	if item == nil {
		return nil
	}
	di := item.(DashboardItem)
	return &di
}

// SelectedProposal returns the proposal at the cursor, or nil.
func (m Model) SelectedProposal() *pipeline.ProposalWithSession {
	item := m.SelectedItem()
	if item == nil || !item.IsProposal() {
		return nil
	}
	return item.Proposal
}

// SelectedFitnessReport returns the fitness report at the cursor, or nil.
func (m Model) SelectedFitnessReport() *fitness.Report {
	item := m.SelectedItem()
	if item == nil || !item.IsFitnessReport() {
		return nil
	}
	return item.FitnessReport
}

// HasActiveInput returns true when the filter input is active.
func (m Model) HasActiveInput() bool {
	return m.list.SettingFilter()
}

// CycleSortOrder advances to the next sort order and refreshes the list.
func (m *Model) CycleSortOrder() tea.Cmd {
	for i, s := range sortOrders {
		if s == m.sortOrder {
			m.sortOrder = sortOrders[(i+1)%len(sortOrders)]
			return m.applySort()
		}
	}
	m.sortOrder = SortNewest
	return m.applySort()
}

// applySort re-sorts m.items and updates the list.
func (m *Model) applySort() tea.Cmd {
	sorted := sortItems(m.items, m.sortOrder)
	return m.list.SetItems(toListItems(sorted))
}

// Reload replaces the proposal list and header stats without resetting cursor, sort order, or filter.
// The existing fitness reports stored on the model are preserved.
// Returns the updated model and any cmd from SetItems (non-nil when a filter is active).
func (m Model) Reload(proposals []pipeline.ProposalWithSession, stats message.DashboardStats) (Model, tea.Cmd) {
	m.stats = stats
	// m.reports is preserved — fitness reports are not reloaded here.
	items := buildItems(proposals, m.reports)
	m.items = items
	sorted := sortItems(items, m.sortOrder)
	cmd := m.list.SetItems(toListItems(sorted))
	return m, cmd
}
