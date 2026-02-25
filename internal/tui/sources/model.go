// Package sources implements the source manager view for the TUI.
// It displays tracked sources grouped by origin, with controls for ownership,
// approach toggling, and change rollback.
package sources

import (
	"time"

	"github.com/charmbracelet/bubbles/viewport"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Model is the source manager view model.
type Model struct {
	groups          []fitness.SourceGroup
	flatItems       []flatItem // flattened for cursor navigation
	cursor          int
	detailOpen      bool
	detailSource    *fitness.Source
	changes         []fitness.ChangeEntry
	confirm         components.ConfirmModel
	confirmState    ConfirmState
	ownershipPrompt string // prompt text for ownership choice (m/n/esc)
	statusMsg       string
	statusExpiry    time.Time
	width           int
	height          int
	viewport        viewport.Model
	keys            *shared.KeyMap
	config          *shared.Config
}

// flatItem maps a visible row to either a group header or source entry.
type flatItem struct {
	isHeader  bool
	groupIdx  int
	sourceIdx int // -1 for headers
}

// ConfirmState tracks which action is pending user confirmation.
type ConfirmState int

const (
	ConfirmNone           ConfirmState = iota
	ConfirmToggleApproach              // awaiting confirm for approach toggle
	ConfirmSetOwnership                // awaiting confirm for ownership change
	ConfirmRollback                    // awaiting confirm for rollback
)

// New creates a source manager model with loaded data.
func New(groups []fitness.SourceGroup, keys *shared.KeyMap, cfg *shared.Config) Model {
	// Apply default collapse state from config.
	for i := range groups {
		groups[i].Collapsed = cfg.SourceManager.GroupCollapsedDefault
	}

	m := Model{
		groups: groups,
		keys:   keys,
		config: cfg,
	}
	m.rebuildFlatItems()
	return m
}

// DetailOpen returns true when the source detail sub-view is open.
func (m Model) DetailOpen() bool { return m.detailOpen }

// HasActivePrompt returns true when the view has internal state
// (confirmation prompt, detail sub-view) that should handle Esc
// before the global handler pops the view.
func (m Model) HasActivePrompt() bool {
	return m.confirm.Active || m.confirmState == ConfirmSetOwnership || m.detailOpen
}

// SetSize updates the viewport dimensions.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	vpH := height - 2 // column header (1) + status bar (1)
	if vpH < 1 {
		vpH = 1
	}
	m.viewport = viewport.New(width, vpH)
	m.refreshViewport()
}

// refreshViewport rebuilds the viewport content from the current flat list.
// Call whenever flatItems, cursor, or group state changes.
func (m *Model) refreshViewport() {
	if m.width == 0 {
		return
	}
	m.viewport.SetContent(m.renderFlatList())
	m.ensureCursorVisible()
}

// ensureCursorVisible adjusts the viewport offset so the cursor row is visible.
func (m *Model) ensureCursorVisible() {
	if len(m.flatItems) == 0 {
		return
	}
	yOff := m.viewport.YOffset
	h := m.viewport.Height
	if m.cursor < yOff {
		m.viewport.YOffset = m.cursor
	} else if m.cursor >= yOff+h {
		m.viewport.YOffset = m.cursor - h + 1
	}
}

// SelectedSource returns the source at the current cursor position,
// or nil if the cursor is on a group header or out of range.
func (m Model) SelectedSource() *fitness.Source {
	if m.cursor < 0 || m.cursor >= len(m.flatItems) {
		return nil
	}
	item := m.flatItems[m.cursor]
	if item.isHeader {
		return nil
	}
	if item.groupIdx < 0 || item.groupIdx >= len(m.groups) {
		return nil
	}
	g := &m.groups[item.groupIdx]
	if item.sourceIdx < 0 || item.sourceIdx >= len(g.Sources) {
		return nil
	}
	s := g.Sources[item.sourceIdx]
	return &s
}

// activeSource returns the source currently being acted on — detailSource
// when the detail sub-view is open, otherwise the cursor-selected source.
func (m Model) activeSource() *fitness.Source {
	if m.detailOpen && m.detailSource != nil {
		return m.detailSource
	}
	return m.SelectedSource()
}

// PreSelectSource returns a new model with cursor positioned on the named source.
func (m Model) PreSelectSource(name string) Model {
	for i, item := range m.flatItems {
		if item.isHeader {
			continue
		}
		g := m.groups[item.groupIdx]
		s := g.Sources[item.sourceIdx]
		if s.Name == name {
			m.cursor = i
			return m
		}
	}
	return m
}

// rebuildFlatItems reconstructs the flat navigation list from the current
// group state. Called after construction and after collapse/expand changes.
func (m *Model) rebuildFlatItems() {
	m.flatItems = nil
	for gi, g := range m.groups {
		// Group header row.
		m.flatItems = append(m.flatItems, flatItem{
			isHeader:  true,
			groupIdx:  gi,
			sourceIdx: -1,
		})
		// Source rows (only when group is expanded).
		if !g.Collapsed {
			for si := range g.Sources {
				m.flatItems = append(m.flatItems, flatItem{
					isHeader:  false,
					groupIdx:  gi,
					sourceIdx: si,
				})
			}
		}
	}
	// Clamp cursor to valid range.
	if m.cursor >= len(m.flatItems) {
		m.cursor = max(0, len(m.flatItems)-1)
	}
	m.refreshViewport()
}

// sourceCounts returns total, iterate, evaluate, and unclassified source counts.
func (m Model) sourceCounts() (total, iterate, evaluate, unclassified int) {
	for _, g := range m.groups {
		for _, s := range g.Sources {
			total++
			switch {
			case s.Ownership == "":
				unclassified++
			case s.Approach == "iterate":
				iterate++
			case s.Approach == "evaluate":
				evaluate++
			}
		}
	}
	return
}
