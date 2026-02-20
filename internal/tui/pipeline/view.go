package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorFgBold)
	sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
	successStyle       = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	errorStyle         = lipgloss.NewStyle().Foreground(shared.ColorError)
	mutedStyle         = lipgloss.NewStyle().Foreground(shared.ColorMuted)
)

type layout int

const (
	layoutNarrow   layout = iota // < 80
	layoutStandard               // 80-119
	layoutWide                   // >= 120
)

func (m Model) layoutMode() layout {
	switch {
	case m.width >= 120:
		return layoutWide
	case m.width >= 80:
		return layoutStandard
	default:
		return layoutNarrow
	}
}

// View renders the pipeline monitor.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var sections []string

	// Title.
	title := titleStyle.Render("Pipeline Monitor")
	sections = append(sections, title)

	// Daemon header.
	sections = append(sections, m.renderDaemonHeader())

	// Activity stats.
	sections = append(sections, m.renderActivityStats())

	// Recent runs.
	sections = append(sections, m.renderRecentRuns())

	// Prompts.
	if len(m.prompts) > 0 {
		sections = append(sections, m.renderPrompts())
	}

	// Confirmation prompt overlay — exclusive return, matching sources pattern.
	if m.confirm.Active {
		return m.confirm.View()
	}

	content := strings.Join(sections, "\n\n")

	// Status bar — 3 args: bindings, timedMsg, width.
	statusBar := components.RenderStatusBar(m.keys.PipelineShortHelp(), "", m.width)

	return content + "\n" + statusBar
}

func (m Model) renderDaemonHeader() string {
	// Build left column: DAEMON section.
	var left strings.Builder
	left.WriteString(sectionHeaderStyle.Render("DAEMON"))
	left.WriteString("\n")
	if m.dashStats.DaemonRunning {
		left.WriteString(fmt.Sprintf("  Status:  %s (PID %d)\n", successStyle.Render("● running"), m.dashStats.DaemonPID))
		if m.dashStats.DaemonStartTime != nil {
			left.WriteString(fmt.Sprintf("  Uptime:  %s\n", formatUptime(time.Since(*m.dashStats.DaemonStartTime))))
		}
	} else {
		left.WriteString(fmt.Sprintf("  Status:  %s\n", errorStyle.Render("● stopped")))
	}
	if m.dashStats.PollInterval > 0 {
		left.WriteString(fmt.Sprintf("  Poll:    every %s\n", formatInterval(m.dashStats.PollInterval)))
		left.WriteString(fmt.Sprintf("  Stale:   every %s\n", formatInterval(m.dashStats.StaleInterval)))
		left.WriteString(fmt.Sprintf("  Delay:   %s", formatInterval(m.dashStats.InterSessionDelay)))
	}

	// Build right column: HOOKS + STORE.
	var right strings.Builder
	right.WriteString(sectionHeaderStyle.Render("HOOKS"))
	right.WriteString("\n")
	right.WriteString(fmt.Sprintf("  pre-compact:  %s\n", checkmark(m.dashStats.HookPreCompact)))
	right.WriteString(fmt.Sprintf("  session-end:  %s\n", checkmark(m.dashStats.HookSessionEnd)))
	right.WriteString("\n")
	right.WriteString(sectionHeaderStyle.Render("STORE"))
	right.WriteString("\n")
	right.WriteString(fmt.Sprintf("  Path: %s\n", m.dashStats.StorePath))
	right.WriteString(fmt.Sprintf("  Raw:  %d sessions\n", m.dashStats.SessionCount))
	right.WriteString(fmt.Sprintf("  Disk: %s", formatBytes(m.dashStats.DiskBytes)))

	// Two-column layout in wide mode, stacked otherwise.
	if m.width >= 120 {
		colWidth := m.width / 2
		leftStyle := lipgloss.NewStyle().Width(colWidth)
		return lipgloss.JoinHorizontal(lipgloss.Top, leftStyle.Render(left.String()), right.String())
	}
	return left.String() + "\n\n" + right.String()
}

// formatUptime formats a duration as "3d 14h 22m".
func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

// formatInterval formats a duration concisely: "2m", "30s", "5m".
func formatInterval(d time.Duration) string {
	if d >= time.Minute {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs > 0 {
			return fmt.Sprintf("%dm%ds", mins, secs)
		}
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(b int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%d MB", b/mb)
	case b >= kb:
		return fmt.Sprintf("%d KB", b/kb)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func (m Model) renderActivityStats() string {
	var b strings.Builder
	days := m.config.Pipeline.SparklineDays
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("PIPELINE ACTIVITY (last %d days)", days)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Sessions captured:  %-6d Proposals generated:  %d\n",
		m.stats.SessionsCaptured, m.stats.ProposalsGenerated))
	b.WriteString(fmt.Sprintf("  Sessions processed: %-6d Proposals approved:   %d\n",
		m.stats.SessionsProcessed, m.stats.ProposalsApproved))
	b.WriteString(fmt.Sprintf("  Sessions pending:   %-6d Proposals rejected:   %d\n",
		m.stats.SessionsPending, m.stats.ProposalsRejected))
	b.WriteString(fmt.Sprintf("  Sessions errored:   %-6d Proposals pending:    %d",
		m.stats.SessionsErrored, m.stats.ProposalsPending))

	if len(m.stats.SessionsPerDay) > 0 {
		sparkline := components.RenderSparkline(m.stats.SessionsPerDay, m.width-4)
		b.WriteString("\n\n  " + sparkline + "  sessions/day")
	}

	return b.String()
}

func (m Model) renderRecentRuns() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("RECENT RUNS"))
	b.WriteString("\n")

	for i, run := range m.runs {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		status := statusIndicator(run.Status)
		shortID := truncateID(run.SessionID)
		age := relativeTime(run.Timestamp)
		project := truncate(run.Project, 16)
		timing := formatTiming(run)

		line := fmt.Sprintf("%s%s %s  %-8s  %-16s  %s", cursor, status, shortID, age, project, timing)
		b.WriteString(line)

		// Inline expansion.
		if i == m.expandedIdx {
			b.WriteString("\n")
			b.WriteString(m.renderRunDetail(run))
		}

		if i < len(m.runs)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m Model) renderRunDetail(run pl.PipelineRun) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("      Session: %s\n", run.SessionID))
	b.WriteString(fmt.Sprintf("      Project: %s\n", run.Project))
	b.WriteString(fmt.Sprintf("      Status:  %s", run.Status))
	if run.ProposalCount > 0 {
		b.WriteString(fmt.Sprintf("\n      Proposals: %d", run.ProposalCount))
	}
	if run.ErrorDetail != "" {
		b.WriteString(fmt.Sprintf("\n      Error: %s", run.ErrorDetail))
	}
	return b.String()
}

func (m Model) renderPrompts() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("PROMPTS"))
	b.WriteString("\n")
	for i, p := range m.prompts {
		age := relativeTime(p.LastUsed)
		b.WriteString(fmt.Sprintf("  %-20s %-4s  last used: %s", p.Name, p.Version, age))
		if i < len(m.prompts)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func statusIndicator(status string) string {
	switch status {
	case "processed":
		return successStyle.Render("✓")
	case "error":
		return errorStyle.Render("✗")
	case "pending":
		return mutedStyle.Render("○")
	default:
		return "?"
	}
}

func checkmark(ok bool) string {
	if ok {
		return successStyle.Render("✓")
	}
	return errorStyle.Render("✗")
}

func formatTiming(run pl.PipelineRun) string {
	if run.Status == "pending" {
		return mutedStyle.Render("(pending)")
	}
	var parts []string
	if run.HasDigest {
		parts = append(parts, fmt.Sprintf("%.1fs parse", run.ParseDuration.Seconds()))
	}
	if run.HasClassifier {
		parts = append(parts, fmt.Sprintf("%.1fs cls", run.ClassifierDuration.Seconds()))
	} else if run.Status == "error" && run.HasDigest {
		parts = append(parts, errorStyle.Render("✗ cls failed"))
	}
	if run.HasEvaluator {
		parts = append(parts, fmt.Sprintf("%.0fs eval", run.EvaluatorDuration.Seconds()))
	} else if run.Status == "error" && run.HasClassifier {
		parts = append(parts, errorStyle.Render("✗ eval failed"))
	}
	return strings.Join(parts, "  ")
}

func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
