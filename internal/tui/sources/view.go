package sources

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"

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


// SubHeader returns the sub-header for the current source view (list or detail).
func (m Model) SubHeader() string {
	if m.detailOpen {
		return m.detailSubHeader()
	}
	return m.renderHeader()
}

// detailSubHeader returns the sub-header for the source detail sub-view.
func (m Model) detailSubHeader() string {
	title := shared.HeaderStyle.Render("  Source Detail")
	if m.detailSource == nil {
		return title
	}
	return title + "\n" + shared.MutedStyle.Render("  "+m.detailSource.Name)
}

// View renders the source manager.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	// Ownership choice prompt overlay.
	if m.confirmState == ConfirmSetOwnership && m.ownershipPrompt != "" {
		return m.ownershipPrompt
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

	// Fixed chrome: column headers (only when items exist).
	if len(m.groups) > 0 {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
	}

	// Scrollable item list.
	if len(m.groups) == 0 {
		b.WriteString("\n")
		b.WriteString(shared.MutedStyle.Render("  No sources tracked."))
		b.WriteString("\n")
		// Fill remaining space.
		content := b.String()
		lines := strings.Count(content, "\n")
		remaining := m.height - lines - 1
		if remaining > 0 {
			content += strings.Repeat("\n", remaining)
		}
		return content + m.renderStatusBar()
	}

	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Status bar.
	return b.String() + m.renderStatusBar()
}

func (m Model) renderHeader() string {
	total, iterate, evaluate, unclassified := m.sourceCounts()

	title := shared.HeaderStyle.Render("  Source Manager")

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

	return title + "\n" + shared.MutedStyle.Render(stats)
}

func (m Model) renderColumnHeaders() string {
	cols := m.columnLayout()

	var parts []string
	parts = append(parts, shared.PadRight("  SOURCE", cols.nameWidth))
	if cols.showOwnership {
		parts = append(parts, shared.PadRight("OWNERSHIP", cols.ownershipWidth))
	}
	parts = append(parts, shared.PadRight("APPROACH", cols.approachWidth))
	parts = append(parts, shared.PadRight("SESSIONS", colSessions))
	if cols.showHealth {
		parts = append(parts, shared.PadRight("HEALTH", colHealth))
	}

	return shared.MutedStyle.Render(strings.Join(parts, "  "))
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
			line = shared.SelectedStyle.Render(line)
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

	return fmt.Sprintf("%s%s %s %s", prefix, chevron, shared.HeaderStyle.Render(g.Label), shared.MutedStyle.Render(count))
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
	name := shared.Truncate(s.Name, cols.nameWidth-4) // account for prefix
	parts = append(parts, prefix+shared.PadRight(name, cols.nameWidth-4))

	// Ownership column (wide/standard only).
	if cols.showOwnership {
		ownership := renderOwnership(s.Ownership)
		parts = append(parts, shared.PadRight(ownership, cols.ownershipWidth))
	}

	// Approach column.
	approach := renderApproach(s.Approach)
	parts = append(parts, shared.PadRight(approach, cols.approachWidth))

	// Sessions column.
	sessions := fmt.Sprintf("%d", s.SessionCount)
	parts = append(parts, shared.PadRight(sessions, colSessions))

	// Health column (wide only).
	if cols.showHealth {
		health := renderHealth(s)
		parts = append(parts, health)
	}

	return strings.Join(parts, "  ")
}

// renderOrigin converts a raw origin string to a display label.
func renderOrigin(origin string) string {
	switch {
	case origin == "user":
		return "User-level"
	case strings.HasPrefix(origin, "project:"):
		return "Project: " + origin[len("project:"):]
	case strings.HasPrefix(origin, "plugin:"):
		return "Plugin: " + origin[len("plugin:"):]
	case origin == "":
		return "unknown"
	default:
		return origin
	}
}

// renderOwnership returns a display string for the ownership field.
func renderOwnership(ownership string) string {
	switch ownership {
	case "mine":
		return "mine"
	case "not_mine":
		return "not mine"
	default:
		return "unknown"
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
		return "not set"
	}
}

// renderHealth returns a health bar string for a source.
func renderHealth(s fitness.Source) string {
	if s.Ownership == "" {
		// Unclassified: no health data.
		return "\u2500\u2500\u2500"
	}

	if s.HealthScore < 0 {
		return shared.MutedStyle.Render("n/a")
	}

	c := healthColor(s.HealthScore, s.Approach)
	bar := components.RenderBar(s.HealthScore, 10, c)
	return fmt.Sprintf("%s %3.0f%%", bar, s.HealthScore)
}

// healthColor returns the bar color for a source health score.
func healthColor(score float64, approach string) color.Color {
	if approach == "iterate" {
		return shared.ColorSuccess
	}
	switch {
	case score >= 80:
		return shared.ColorSuccess
	case score >= 50:
		return shared.ColorWarning
	default:
		return shared.ColorError
	}
}

// renderHealthText returns a plain-text health summary for the detail info section.
func renderHealthText(s *fitness.Source) string {
	if s.Ownership == "" {
		return "n/a (unclassified)"
	}
	if s.HealthScore < 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", s.HealthScore)
}

// Detail sub-view.

func (m Model) renderDetail() string {
	if m.detailSource == nil {
		return ""
	}
	src := m.detailSource

	var b strings.Builder

	// Source info.
	b.WriteString("  " + shared.AccentBoldStyle.Render("INFO") + "\n\n")
	b.WriteString(fmt.Sprintf("  %-12s %s\n", shared.MutedStyle.Render("Origin:"), renderOrigin(src.Origin)))
	b.WriteString(fmt.Sprintf("  %-12s %s\n", shared.MutedStyle.Render("Ownership:"), renderOwnership(src.Ownership)))
	b.WriteString(fmt.Sprintf("  %-12s %s\n", shared.MutedStyle.Render("Approach:"), renderApproach(src.Approach)))
	b.WriteString(fmt.Sprintf("  %-12s %d\n", shared.MutedStyle.Render("Sessions:"), src.SessionCount))
	b.WriteString(fmt.Sprintf("  %-12s %s\n", shared.MutedStyle.Render("Health:"), renderHealthText(src)))
	if src.ClassifiedAt != nil {
		b.WriteString(fmt.Sprintf("  %-12s %s\n", shared.MutedStyle.Render("Classified:"), src.ClassifiedAt.Format("2006-01-02 15:04")))
	}
	b.WriteString("\n")

	// Recent changes.
	b.WriteString("  " + shared.AccentBoldStyle.Render("RECENT CHANGES") + "\n\n")

	if len(m.changes) == 0 {
		b.WriteString(shared.MutedStyle.Render("  No changes recorded.") + "\n")
	} else {
		for _, c := range m.changes {
			status := shared.AccentStyle.Render(c.Status)
			if c.Status == "approved" {
				status = shared.SuccessStyle.Render(c.Status)
			} else if c.Status == "rejected" {
				status = shared.ErrorStyle.Render(c.Status)
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				shared.MutedStyle.Render(c.Timestamp.Format("2006-01-02 15:04")),
				status,
				c.Description,
			))
		}
		b.WriteString("\n")
		b.WriteString(shared.MutedStyle.Render("  Press z to rollback the most recent change.") + "\n")
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
	return components.RenderStatusBar(m.keys.SourcesShortHelp(), m.statusMsg, m.width)
}

func (m Model) renderDetailStatusBar() string {
	bindings := []key.Binding{m.keys.SetOwnership, m.keys.ToggleApproach, m.keys.Rollback, m.keys.Back, m.keys.Help}
	return components.RenderStatusBar(bindings, m.statusMsg, m.width)
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

