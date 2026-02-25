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
	if m.list.Width() == 0 || m.list.Height() == 0 {
		return ""
	}

	var b strings.Builder

	// Fixed chrome above list: column headers (only when items exist).
	if len(m.list.Items()) > 0 {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
	}

	// Scrollable list (handles empty state internally via custom rendering).
	if len(m.list.VisibleItems()) == 0 && !m.list.SettingFilter() {
		// No items — show flavor text padded to the list height so the view
		// fills the terminal exactly as the list would when items are present.
		// The empty block does NOT append a trailing \n; it's self-contained.
		emptyLine := "\n" + shared.MutedStyle.Render("  "+components.EmptyProposals())
		listHeight := m.list.Height()
		padding := listHeight - strings.Count(emptyLine, "\n") - 1
		if padding > 0 {
			emptyLine += strings.Repeat("\n", padding)
		}
		b.WriteString(emptyLine)
		b.WriteString("\n") // terminates the block (equivalent to list.View() + "\n")
	} else {
		b.WriteString(m.list.View())
		b.WriteString("\n")
	}

	// Sort indicator + filter status (only when items exist and not in filter input mode).
	if len(m.list.Items()) > 0 && !m.list.SettingFilter() {
		sortLine := fmt.Sprintf("  Sort: %s", m.sortOrder)
		if m.list.IsFiltered() {
			sortLine += fmt.Sprintf("  ·  filter: %q  (%d/%d)",
				m.list.FilterValue(),
				len(m.list.VisibleItems()),
				len(m.list.Items()))
		}
		b.WriteString(shared.MutedStyle.Render(sortLine))
		b.WriteString("\n")
	}

	// Filter input or status bar.
	if m.list.SettingFilter() {
		b.WriteString("/ " + m.list.FilterInput.View())
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
	cols := columnLayoutForWidth(m.list.Width())
	header := shared.PadRight("   TYPE", cols.typeWidth+3) +
		"  " + shared.PadRight("TARGET", cols.targetWidth) +
		"  " + "CONFIDENCE"
	return shared.MutedStyle.Render(header)
}

func (m Model) renderStatusBar() string {
	if len(m.list.VisibleItems()) == 0 {
		bindings := []key.Binding{m.keys.Sources, m.keys.Pipeline, m.keys.Help}
		return components.RenderStatusBar(bindings, m.statusMsg, m.list.Width())
	}
	item := m.SelectedItem()
	if item != nil && item.IsFitnessReport() {
		bindings := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Open, m.keys.Sources, m.keys.Help}
		return components.RenderStatusBar(bindings, m.statusMsg, m.list.Width())
	}
	return components.RenderStatusBar(m.keys.ShortHelp(), m.statusMsg, m.list.Width())
}

// viewportHeight returns the list height accounting for chrome.
func (m Model) viewportHeight(width, height int) int {
	chrome := 1 // status/filter bar
	if len(m.list.Items()) > 0 {
		chrome += 2 // column headers + sort indicator
	}
	h := height - chrome
	if h < 1 {
		h = 1
	}
	return h
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

// columnLayoutForWidth computes column widths for the given terminal width.
func columnLayoutForWidth(width int) columnSpec {
	// Row: prefix(2) + " " + indicator(1) + " " + type + "  " + target + "  " + confidence
	// Fixed overhead = 5 + typeWidth + 2 + 2 + confidenceWidth
	overhead := 5 + colType + 2 + 2 + colConfidence
	targetWidth := width - overhead
	if targetWidth < 15 {
		targetWidth = 15
	}
	return columnSpec{typeWidth: colType, targetWidth: targetWidth}
}
