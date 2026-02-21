package fitness

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	fitnessHeader  = lipgloss.NewStyle().Bold(true)
	fitnessMuted   = lipgloss.NewStyle().Foreground(shared.ColorMuted)
	fitnessSection = lipgloss.NewStyle().Bold(true)
	fitnessAccent  = lipgloss.NewStyle().Foreground(shared.ColorAccent)
)

// View renders the fitness report detail view.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.report == nil {
		return "  No report selected."
	}

	var b strings.Builder

	// Header: source name, ownership, mode, observed count.
	r := m.report
	b.WriteString(fitnessHeader.Render(fmt.Sprintf("  Fitness Report: %s", r.SourceName)))
	b.WriteString("\n")
	ownershipLine := fmt.Sprintf("  Ownership: %s  |  Origin: %s  |  Observed: %d sessions (%d days)",
		fitnessAccent.Render(r.Ownership),
		fitnessMuted.Render(r.SourceOrigin),
		r.ObservedCount,
		r.WindowDays)
	// Split ownership/origin onto separate lines when the combined line would overflow.
	if m.width < 100 {
		ownershipLine = fmt.Sprintf("  Ownership: %s  |  Origin: %s\n  Observed: %d sessions (%d days)",
			fitnessAccent.Render(r.Ownership),
			fitnessMuted.Render(r.SourceOrigin),
			r.ObservedCount,
			r.WindowDays)
	}
	b.WriteString(ownershipLine + "\n")
	b.WriteString("\n")

	// Viewport with assessment, verdict, and evidence.
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	remaining := m.height - lines - 1
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar.
	content += components.RenderStatusBar(m.keys.FitnessShortHelp(), "", m.width)

	return content
}

// renderViewportContent builds the scrollable content for the viewport.
// Called whenever evidence expand state changes.
func (m Model) renderViewportContent() string {
	if m.report == nil {
		return ""
	}

	var b strings.Builder

	// ASSESSMENT section.
	b.WriteString(fitnessSection.Render("  ASSESSMENT"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("\u2500", 17))
	b.WriteString("\n")

	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	b.WriteString(components.RenderAssessment(m.report.Assessment, contentWidth))
	b.WriteString("\n\n")

	// VERDICT section.
	b.WriteString(fitnessSection.Render("  VERDICT"))
	b.WriteString("\n")
	b.WriteString("  " + strings.Repeat("\u2500", 17))
	b.WriteString("\n")
	b.WriteString(shared.WrapIndent(m.report.Verdict, m.viewport.Width, 2))
	b.WriteString("\n\n")

	// SESSION EVIDENCE section.
	if len(m.evidence) > 0 {
		b.WriteString(fitnessSection.Render(fmt.Sprintf("  SESSION EVIDENCE (%d groups)", len(m.evidence))))
		b.WriteString("\n")
		b.WriteString("  " + strings.Repeat("\u2500", 17))
		b.WriteString("\n")
		b.WriteString(m.renderEvidence())
	}

	return b.String()
}

// renderEvidence renders the evidence groups with expand/collapse support.
func (m Model) renderEvidence() string {
	var b strings.Builder

	for i, eg := range m.evidence {
		isCursor := i == m.selectedEvidence

		// Group header with chevron and entry count.
		prefix := "  "
		if isCursor {
			prefix = "> "
		}
		chevron := "\u25b6" // collapsed
		if eg.Expanded {
			chevron = "\u25bc" // expanded
		}

		categoryLabel := formatCategory(eg.Category)
		noun := "entries"
		if len(eg.Entries) == 1 {
			noun = "entry"
		}
		countLabel := fmt.Sprintf("(%d %s)", len(eg.Entries), noun)

		b.WriteString(fmt.Sprintf("  %s%s %s %s\n",
			prefix,
			chevron,
			fitnessAccent.Render(categoryLabel),
			fitnessMuted.Render(countLabel)))

		// Expanded entries.
		if eg.Expanded {
			for _, entry := range eg.Entries {
				ts := entry.Timestamp.Format("2006-01-02 15:04")
				b.WriteString(fmt.Sprintf("      %s  %s\n",
					fitnessMuted.Render(ts),
					entry.Summary))
				if entry.Detail != "" {
					b.WriteString(shared.IndentBlock(entry.Detail, 8))
					b.WriteString("\n")
				}
			}
		}

		b.WriteString("\n")
	}

	return b.String()
}

// formatCategory returns a human-friendly label for an evidence category.
func formatCategory(category string) string {
	switch category {
	case "followed":
		return "Followed Correctly"
	case "worked_around":
		return "Worked Around"
	case "confused":
		return "Confused"
	default:
		return category
	}
}

