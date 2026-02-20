package sources

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Layout breakpoints.
const (
	breakpointWide     = 120
	breakpointStandard = 80
)

// Column widths.
const (
	colSessions = 10
	colHealth   = 16
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(shared.ColorMuted)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorFgBold)
	successStyle  = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	warningStyle  = lipgloss.NewStyle().Foreground(shared.ColorWarning)
	accentStyle   = lipgloss.NewStyle().Foreground(shared.ColorAccent)
	errorStyle    = lipgloss.NewStyle().Foreground(shared.ColorError)
)

// View renders the source manager.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Confirmation prompt overlay.
	if m.confirm.Active {
		return m.confirm.View()
	}

	// Detail sub-view.
	if m.detailOpen {
		return m.renderDetail()
	}

	var b strings.Builder

	// Header with source counts.
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Separator.
	b.WriteString(strings.Repeat("\u2500", m.width))
	b.WriteString("\n")

	// Column headers.
	b.WriteString(m.renderColumnHeaders())
	b.WriteString("\n")

	// Content.
	if len(m.groups) == 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  No sources tracked."))
		b.WriteString("\n")
	} else {
		b.WriteString(m.renderFlatList())
	}

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	statusBarHeight := 1
	remaining := m.height - lines - statusBarHeight
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar.
	content += m.renderStatusBar()

	return content
}

func (m Model) renderHeader() string {
	total, iterate, evaluate, unclassified := m.sourceCounts()

	title := headerStyle.Render("  Source Manager")

	stats := fmt.Sprintf("  %d sources", total)
	if iterate > 0 {
		stats += fmt.Sprintf("  \u00b7  %d iterate", iterate)
	}
	if evaluate > 0 {
		stats += fmt.Sprintf("  \u00b7  %d evaluate", evaluate)
	}
	if unclassified > 0 {
		stats += fmt.Sprintf("  \u00b7  %d unclassified", unclassified)
	}

	return title + "\n" + mutedStyle.Render(stats)
}

func (m Model) renderColumnHeaders() string {
	cols := m.columnLayout()

	var parts []string
	parts = append(parts, padRight("  SOURCE", cols.nameWidth))
	if cols.showOwnership {
		parts = append(parts, padRight("OWNERSHIP", cols.ownershipWidth))
	}
	parts = append(parts, padRight("APPROACH", cols.approachWidth))
	parts = append(parts, padRight("SESSIONS", colSessions))
	if cols.showHealth {
		parts = append(parts, padRight("HEALTH", colHealth))
	}

	return mutedStyle.Render(strings.Join(parts, "  "))
}

func (m Model) renderFlatList() string {
	var b strings.Builder
	cols := m.columnLayout()

	for i, item := range m.flatItems {
		isCursor := i == m.cursor

		var line string
		if item.isHeader {
			line = m.renderGroupHeader(item.groupIdx, isCursor)
		} else {
			line = m.renderSourceRow(item.groupIdx, item.sourceIdx, isCursor, cols)
		}

		if isCursor {
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderGroupHeader(gi int, isCursor bool) string {
	g := m.groups[gi]

	prefix := "  "
	if isCursor {
		prefix = "> "
	}

	chevron := "\u25bc" // expanded
	if g.Collapsed {
		chevron = "\u25b6" // collapsed
	}

	count := fmt.Sprintf("(%d)", len(g.Sources))

	return fmt.Sprintf("%s%s %s %s", prefix, chevron, headerStyle.Render(g.Label), mutedStyle.Render(count))
}

func (m Model) renderSourceRow(gi, si int, isCursor bool, cols columnSpec) string {
	g := m.groups[gi]
	s := g.Sources[si]

	prefix := "    " // indented under group header
	if isCursor {
		prefix = " >  "
	}

	var parts []string

	// Name column.
	name := truncate(s.Name, cols.nameWidth-4) // account for prefix
	parts = append(parts, prefix+padRight(name, cols.nameWidth-4))

	// Ownership column (wide/standard only).
	if cols.showOwnership {
		ownership := renderOwnership(s.Ownership)
		parts = append(parts, padRight(ownership, cols.ownershipWidth))
	}

	// Approach column.
	approach := renderApproach(s.Approach)
	parts = append(parts, padRight(approach, cols.approachWidth))

	// Sessions column.
	sessions := fmt.Sprintf("%d", s.SessionCount)
	parts = append(parts, padRight(sessions, colSessions))

	// Health column (wide only).
	if cols.showHealth {
		health := renderHealth(s)
		parts = append(parts, health)
	}

	return strings.Join(parts, "  ")
}

// renderOwnership returns a display string for the ownership field.
func renderOwnership(ownership string) string {
	switch ownership {
	case "mine":
		return "mine"
	case "not_mine":
		return "not mine"
	default:
		return "\u26a0"
	}
}

// renderApproach returns a display string for the approach field.
func renderApproach(approach string) string {
	switch approach {
	case "iterate":
		return "iterate"
	case "evaluate":
		return "evaluate"
	case "paused":
		return "paused"
	default:
		return "-"
	}
}

// renderHealth returns a health bar string for a source.
func renderHealth(s fitness.Source) string {
	if s.Ownership == "" {
		// Unclassified: no health data.
		return "\u2500\u2500\u2500"
	}

	if s.HealthScore < 0 {
		return mutedStyle.Render("n/a")
	}

	return renderBar(s.HealthScore, s.Approach)
}

// renderBar renders a text-based progress bar.
// For iterate sources: green filled bar (approval ratio).
// For evaluate sources: colored by score thresholds.
func renderBar(score float64, approach string) string {
	const barWidth = 10
	filled := int(score / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	if filled < 0 {
		filled = 0
	}

	empty := barWidth - filled

	var barStyle lipgloss.Style
	switch {
	case approach == "iterate":
		barStyle = successStyle
	case score >= 80:
		barStyle = successStyle
	case score >= 50:
		barStyle = warningStyle
	default:
		barStyle = errorStyle
	}

	bar := barStyle.Render(strings.Repeat("\u2588", filled)) +
		mutedStyle.Render(strings.Repeat("\u2591", empty))

	return fmt.Sprintf("%s %3.0f%%", bar, score)
}

// Detail sub-view.

func (m Model) renderDetail() string {
	if m.detailSource == nil {
		return ""
	}

	var b strings.Builder

	s := m.detailSource

	b.WriteString(headerStyle.Render("  Source: "+s.Name) + "\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Origin: %s  |  Ownership: %s  |  Approach: %s",
		s.Origin, renderOwnership(s.Ownership), renderApproach(s.Approach))) + "\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Sessions: %d  |  Health: %.0f%%",
		s.SessionCount, s.HealthScore)) + "\n")
	b.WriteString("\n")
	b.WriteString(strings.Repeat("\u2500", m.width) + "\n")

	// Recent changes.
	b.WriteString(headerStyle.Render("  Recent Changes") + "\n\n")

	if len(m.changes) == 0 {
		b.WriteString(mutedStyle.Render("  No changes recorded.") + "\n")
	} else {
		for _, c := range m.changes {
			status := accentStyle.Render(c.Status)
			if c.Status == "approved" {
				status = successStyle.Render(c.Status)
			} else if c.Status == "rejected" {
				status = errorStyle.Render(c.Status)
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				mutedStyle.Render(c.Timestamp.Format("2006-01-02 15:04")),
				status,
				c.Description,
			))
		}
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  Press z to rollback the most recent change.") + "\n")
	}

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	statusBarHeight := 1
	remaining := m.height - lines - statusBarHeight
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar.
	content += m.renderDetailStatusBar()

	return content
}

func (m Model) renderStatusBar() string {
	return components.RenderStatusBar(m.keys.SourcesShortHelp(), "", m.width)
}

func (m Model) renderDetailStatusBar() string {
	bindings := []key.Binding{m.keys.Back, m.keys.Rollback, m.keys.Help}
	return components.RenderStatusBar(bindings, "", m.width)
}

// Column layout.

type columnSpec struct {
	nameWidth      int
	ownershipWidth int
	approachWidth  int
	showOwnership  bool
	showHealth     bool
}

func (m Model) columnLayout() columnSpec {
	if m.width >= breakpointWide {
		// Wide: all 5 columns.
		return columnSpec{
			nameWidth:      m.width - colSessions - colHealth - 12 - 10 - 12, // gaps + ownership + approach
			ownershipWidth: 12,
			approachWidth:  10,
			showOwnership:  true,
			showHealth:     true,
		}
	}
	if m.width >= breakpointStandard {
		// Standard: 4 columns (no health).
		return columnSpec{
			nameWidth:      m.width - colSessions - 12 - 10 - 8, // gaps + ownership + approach
			ownershipWidth: 12,
			approachWidth:  10,
			showOwnership:  true,
			showHealth:     false,
		}
	}
	// Narrow: 3 columns (name, approach, sessions).
	return columnSpec{
		nameWidth:     m.width - colSessions - 10 - 6, // gaps + approach
		approachWidth: 10,
		showOwnership: false,
		showHealth:    false,
	}
}

// Helpers.

func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func padRight(s string, width int) string {
	if width <= 0 {
		return s
	}
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
