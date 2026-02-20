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

	// Confirm overlay.
	if m.confirm.Active {
		sections = append(sections, m.confirm.View())
	}

	content := strings.Join(sections, "\n\n")

	// Status bar — 3 args: bindings, timedMsg, width.
	statusBar := components.RenderStatusBar(m.keys.PipelineShortHelp(), "", m.width)

	return content + "\n" + statusBar
}

func (m Model) renderDaemonHeader() string {
	var b strings.Builder

	b.WriteString(sectionHeaderStyle.Render("DAEMON"))
	b.WriteString("\n")

	if m.dashStats.DaemonRunning {
		b.WriteString(fmt.Sprintf("  Status:  %s (PID %d)\n", successStyle.Render("● running"), m.dashStats.DaemonPID))
	} else {
		b.WriteString(fmt.Sprintf("  Status:  %s\n", errorStyle.Render("● stopped")))
	}

	b.WriteString("\n")

	b.WriteString(sectionHeaderStyle.Render("HOOKS"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  pre-compact:  %s\n", checkmark(m.dashStats.HookPreCompact)))
	b.WriteString(fmt.Sprintf("  session-end:  %s", checkmark(m.dashStats.HookSessionEnd)))

	return b.String()
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
		shortID := run.SessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}
