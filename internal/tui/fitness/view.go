package fitness

import (
	"fmt"
	"strings"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)


// SubHeader returns the view title and contextual stats for the fitness report.
func (m Model) SubHeader() string {
	if m.report == nil {
		return shared.RenderSubHeader("  Fitness Report", "")
	}
	r := m.report
	statsLine := fmt.Sprintf("  %s  ·  ownership: %s  ·  %d sessions",
		r.SourceName, r.Ownership, r.ObservedCount)
	return shared.RenderSubHeader("  Fitness Report", statsLine)
}

// View renders the fitness report detail view.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	if m.report == nil {
		return "  No report selected."
	}

	var b strings.Builder

	b.WriteString("\n") // spacing before viewport

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
	content += components.RenderStatusBar(m.keys.FitnessShortHelp(), m.statusMsg, m.width)

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
	b.WriteString(shared.RenderSectionHeader("ASSESSMENT"))
	b.WriteString("\n")

	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	b.WriteString(components.RenderAssessment(m.report.Assessment, contentWidth))
	b.WriteString("\n\n")

	// VERDICT section.
	b.WriteString(shared.RenderSectionHeader("VERDICT"))
	b.WriteString("\n")
	b.WriteString(shared.WrapIndent(m.report.Verdict, m.viewport.Width(), 2))
	b.WriteString("\n\n")

	// SESSION EVIDENCE section.
	if len(m.evidence) > 0 {
		b.WriteString(shared.RenderSectionHeader(fmt.Sprintf("SESSION EVIDENCE (%d groups)", len(m.evidence))))
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
			shared.AccentStyle.Render(categoryLabel),
			shared.MutedStyle.Render(countLabel)))

		// Expanded entries.
		if eg.Expanded {
			for _, entry := range eg.Entries {
				ts := entry.Timestamp.Format("2006-01-02 15:04")
				b.WriteString(fmt.Sprintf("      %s  %s\n",
					shared.MutedStyle.Render(ts),
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

