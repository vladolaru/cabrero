// Package fitness implements the fitness report detail view for the TUI.
// It displays a fitness assessment of a third-party artifact, showing health
// metrics (assessment bars), a verdict, and session evidence grouped by category.
package fitness

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Focus identifies which pane has focus in the fitness detail view.
type Focus int

const (
	FocusReport Focus = iota
	FocusChat
)

// Model is the fitness report detail view model.
type Model struct {
	report           *fitness.Report
	viewport         viewport.Model
	evidence         []fitness.EvidenceGroup
	selectedEvidence int // cursor for evidence groups
	focus            Focus
	statusMsg        string
	statusExpiry     time.Time
	spinner          spinner.Model
	width            int
	height           int
	keys             *shared.KeyMap
	config           *shared.Config
}

// New creates a fitness detail model for the given report.
func New(report *fitness.Report, keys *shared.KeyMap, cfg *shared.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	// Copy evidence groups so toggling Expanded doesn't mutate the original.
	var evidence []fitness.EvidenceGroup
	if report != nil {
		evidence = make([]fitness.EvidenceGroup, len(report.Evidence))
		for i, eg := range report.Evidence {
			entries := make([]fitness.EvidenceEntry, len(eg.Entries))
			copy(entries, eg.Entries)
			evidence[i] = fitness.EvidenceGroup{
				Category: eg.Category,
				Entries:  entries,
				Expanded: eg.Expanded,
			}
		}
	}

	m := Model{
		report:   report,
		evidence: evidence,
		focus:    FocusReport,
		spinner:  s,
		keys:     keys,
		config:   cfg,
	}

	return m
}

// SetSize updates the model dimensions and recalculates the viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Reserve space for header (4 lines) and status bar (1 line).
	contentHeight := height - 5
	if contentHeight < 4 {
		contentHeight = 4
	}

	contentWidth := width - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	m.viewport = viewport.New(contentWidth, contentHeight)
	m.viewport.SetContent(m.renderViewportContent())
}

