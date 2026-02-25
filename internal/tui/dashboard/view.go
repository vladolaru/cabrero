package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Type indicator characters.
const (
	indicatorProposal = "●"
	indicatorFitness  = "◎"
)

// View renders the dashboard.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Fixed chrome above viewport: column headers (only when items exist).
	if len(m.filtered) > 0 {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
	}

	// Scrollable item rows.
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Fixed chrome below viewport: sort indicator (only when items exist).
	if len(m.filtered) > 0 {
		b.WriteString(shared.MutedStyle.Render(fmt.Sprintf("  Sort: %s", m.sortOrder)))
		b.WriteString("\n")
	}

	// Filter bar or status bar.
	if m.filterActive {
		b.WriteString(m.filterInput.View())
	} else {
		b.WriteString(m.renderStatusBar())
	}

	return b.String()
}

// SubHeader returns the view title and stats line for the dashboard.
func (m Model) SubHeader() string {
	statsLine := fmt.Sprintf("  %d awaiting review", m.stats.PendingCount)
	if m.stats.ApprovedCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d approved", m.stats.ApprovedCount)
	}
	if m.stats.RejectedCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d rejected", m.stats.RejectedCount)
	}
	if m.stats.FitnessReportCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d fitness reports", m.stats.FitnessReportCount)
	}
	return shared.RenderSubHeader("  Proposals", statsLine)
}

func (m Model) renderColumnHeaders() string {
	cols := m.columnLayout()

	// Row: prefix(2) + " " + indicator(1) + " " + type(18) + "  " + target + "  " + confidence
	// TYPE aligns with the bullet indicator at position 3 (prefix + space).
	header := shared.PadRight("   TYPE", cols.typeWidth+3) +
		"  " + shared.PadRight("TARGET", cols.targetWidth) +
		"  " + "CONFIDENCE"

	return shared.MutedStyle.Render(header)
}

func (m Model) renderItemRows() string {
	var b strings.Builder
	cols := m.columnLayout()

	for i, item := range m.filtered {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		// Choose indicator style based on item type.
		var indicator string
		if item.IsProposal() {
			indicator = shared.AccentStyle.Render(indicatorProposal)
		} else {
			indicator = shared.WarningStyle.Render(indicatorFitness)
		}

		typeName := shared.PadRight(item.TypeName(), cols.typeWidth)
		target := shared.TruncatePad(shared.ShortenHome(item.Target()), cols.targetWidth)
		confidence := shared.MutedStyle.Render(item.Confidence())

		line := fmt.Sprintf("%s %s %s  %s  %s", prefix, indicator, typeName, target, confidence)

		if i == m.cursor {
			line = shared.SelectedStyle.Render(line)
		}

		b.WriteString(line)
		if i < len(m.filtered)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderStatusBar() string {
	keys := m.keys

	// Empty state: show only navigation keys that make sense.
	if len(m.filtered) == 0 {
		bindings := []key.Binding{keys.Sources, keys.Pipeline, keys.Help}
		return components.RenderStatusBar(bindings, m.statusMsg, m.width)
	}

	// Show different actions depending on the selected item type.
	item := m.SelectedItem()
	if item != nil && item.IsFitnessReport() {
		bindings := []key.Binding{
			keys.Up, keys.Down, keys.Open, keys.Sources, keys.Help,
		}
		return components.RenderStatusBar(bindings, m.statusMsg, m.width)
	}

	return components.RenderStatusBar(keys.ShortHelp(), m.statusMsg, m.width)
}

// Column layout.

const (
	colType       = 18 // longest type: "skill_improvement" = 17 + 1
	colConfidence = 12 // longest: "100% health" = 11 + 1
)

type columnSpec struct {
	typeWidth   int
	targetWidth int
}

func (m Model) columnLayout() columnSpec {
	// Row: prefix(2) + " " + indicator(1) + " " + type + "  " + target + "  " + confidence
	// Fixed overhead = 5 + typeWidth + 2 + 2 + confidenceWidth
	overhead := 5 + colType + 2 + 2 + colConfidence
	targetWidth := m.width - overhead
	if targetWidth < 15 {
		targetWidth = 15
	}
	return columnSpec{typeWidth: colType, targetWidth: targetWidth}
}

