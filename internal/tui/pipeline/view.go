package pipeline

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	pl "github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/store"
	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorFgBold)
	sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(shared.ColorAccent)
	successStyle       = shared.SuccessStyle
	warningStyle       = shared.WarningStyle
	errorStyle         = shared.ErrorStyle
	mutedStyle         = shared.MutedStyle
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

	// Models: hidden in narrow mode.
	if m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderModels())
	}

	// Prompts: hidden in narrow mode.
	if len(m.prompts) > 0 && m.layoutMode() != layoutNarrow {
		sections = append(sections, m.renderPrompts())
	}

	// Confirmation prompt overlay — exclusive return, matching sources pattern.
	if m.confirm.Active {
		return m.confirm.View()
	}

	content := strings.Join(sections, "\n\n")

	// Fill remaining space to anchor status bar to bottom.
	lines := strings.Count(content, "\n")
	statusBarHeight := 1
	remaining := m.height - lines - statusBarHeight
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Status bar — 3 args: bindings, timedMsg, width.
	statusBar := components.RenderStatusBar(m.keys.PipelineShortHelp(), m.statusMsg, m.width)

	return content + statusBar
}

func (m Model) renderDaemonHeader() string {
	mode := m.layoutMode()

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
	// Intervals: shown in standard and wide, omitted in narrow.
	if mode != layoutNarrow && m.dashStats.PollInterval > 0 {
		left.WriteString(fmt.Sprintf("  Poll:    every %s\n", formatInterval(m.dashStats.PollInterval)))
		left.WriteString(fmt.Sprintf("  Stale:   every %s\n", formatInterval(m.dashStats.StaleInterval)))
		left.WriteString(fmt.Sprintf("  Delay:   %s", formatInterval(m.dashStats.InterSessionDelay)))
	}
	if m.dashStats.DebugMode {
		left.WriteString(fmt.Sprintf("\n  Debug:   %s", warningStyle.Render("enabled")))
	}

	// Build right column: HOOKS + STORE (store hidden in narrow).
	var right strings.Builder
	right.WriteString(sectionHeaderStyle.Render("HOOKS"))
	right.WriteString("\n")
	right.WriteString(fmt.Sprintf("  pre-compact:  %s\n", checkmark(m.dashStats.HookPreCompact)))
	right.WriteString(fmt.Sprintf("  session-end:  %s", checkmark(m.dashStats.HookSessionEnd)))

	if mode != layoutNarrow {
		right.WriteString("\n\n")
		right.WriteString(sectionHeaderStyle.Render("STORE"))
		right.WriteString("\n")
		right.WriteString(fmt.Sprintf("  Path: %s\n", m.dashStats.StorePath))
		right.WriteString(fmt.Sprintf("  Raw:  %d sessions\n", m.dashStats.SessionCount))
		right.WriteString(fmt.Sprintf("  Disk: %s", formatBytes(m.dashStats.DiskBytes)))
	}

	// Layout: wide = two-column, standard/narrow = stacked.
	if mode == layoutWide {
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
	mode := m.layoutMode()
	var b strings.Builder
	days := m.config.Pipeline.SparklineDays
	b.WriteString(sectionHeaderStyle.Render(fmt.Sprintf("PIPELINE ACTIVITY (last %d days)", days)))
	b.WriteString("\n")

	if mode == layoutNarrow {
		// Stacked: one stat per line.
		b.WriteString(fmt.Sprintf("  Captured:  %d   Processed: %d\n", m.stats.SessionsCaptured, m.stats.SessionsProcessed))
		b.WriteString(fmt.Sprintf("  Queued:    %d   Errored:   %d\n", m.stats.SessionsQueued, m.stats.SessionsErrored))
		b.WriteString(fmt.Sprintf("  Proposals: %d gen  %d ok  %d rej\n", m.stats.ProposalsGenerated, m.stats.ProposalsApproved, m.stats.ProposalsRejected))
		b.WriteString(fmt.Sprintf("  Tokens:    %s in / %s out  %s",
			formatTokenCount(m.stats.TotalInputTokens),
			formatTokenCount(m.stats.TotalOutputTokens),
			formatCost(m.stats.TotalCostUSD)))
	} else {
		// Wide/standard: 2-column layout.
		b.WriteString(fmt.Sprintf("  Sessions captured:  %-6d Proposals generated:  %d\n",
			m.stats.SessionsCaptured, m.stats.ProposalsGenerated))
		b.WriteString(fmt.Sprintf("  Sessions processed: %-6d Proposals approved:   %d\n",
			m.stats.SessionsProcessed, m.stats.ProposalsApproved))
		b.WriteString(fmt.Sprintf("  Sessions queued:    %-6d Proposals rejected:   %d\n",
			m.stats.SessionsQueued, m.stats.ProposalsRejected))
		b.WriteString(fmt.Sprintf("  Sessions errored:   %-6d Proposals pending:    %d\n",
			m.stats.SessionsErrored, m.stats.ProposalsPending))
		b.WriteString(fmt.Sprintf("  Tokens: %s in / %s out         Cost: %s",
			formatTokenCount(m.stats.TotalInputTokens),
			formatTokenCount(m.stats.TotalOutputTokens),
			formatCost(m.stats.TotalCostUSD)))

		if len(m.stats.SessionsPerDay) > 0 {
			sparkline := components.RenderSparkline(m.stats.SessionsPerDay, m.width-4)
			b.WriteString("\n\n  " + sparkline + "  sessions/day")
		}
	}

	return b.String()
}

// formatTokenCount formats a token count as a compact string: "0", "1.2K", "45.6K", "1.2M".
func formatTokenCount(tokens int) string {
	switch {
	case tokens >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(tokens)/1_000_000)
	case tokens >= 1_000:
		return fmt.Sprintf("%.1fK", float64(tokens)/1_000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}

// formatCost formats a USD cost as "$0.42" or "$0.00" when zero.
func formatCost(cost float64) string {
	return fmt.Sprintf("$%.2f", cost)
}

// runLayout returns the display parameters for a run row based on terminal width.
func (m Model) runLayout() (idLen int, projectMax int) {
	switch m.layoutMode() {
	case layoutWide:
		return 8, 20
	case layoutStandard:
		return 8, 15
	default: // narrow
		return 8, 10
	}
}

func (m Model) renderRecentRuns() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("RECENT RUNS"))
	b.WriteString("\n")

	idLen, projectMax := m.runLayout()
	mode := m.layoutMode()

	for i, run := range m.runs {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		status := statusIndicator(run.Status)
		shortID := store.ShortSessionID(run.SessionID)
		age := relativeTime(run.Timestamp)
		project := shared.Truncate(run.Project, projectMax)
		timing := formatTimingForMode(run, mode)

		line := fmt.Sprintf("%s%s %-*s  %-8s  %-*s  %s", cursor, status, idLen, shortID, age, projectMax, project, timing)
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
	b.WriteString(fmt.Sprintf("      Session:    %s\n", run.SessionID))
	b.WriteString(fmt.Sprintf("      Project:    %s\n", run.Project))
	b.WriteString(fmt.Sprintf("      Status:     %s", run.Status))
	if run.ProposalCount > 0 {
		b.WriteString(fmt.Sprintf("\n      Proposals:  %d", run.ProposalCount))
	}
	if run.ErrorDetail != "" {
		b.WriteString(fmt.Sprintf("\n      Error:      %s", run.ErrorDetail))
	}
	return b.String()
}

func (m Model) renderModels() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("MODELS"))
	b.WriteString("\n")

	cfg := pl.DefaultPipelineConfig()
	b.WriteString(fmt.Sprintf("  Classifier:  %s", cfg.ClassifierModel))
	if cfg.ClassifierModel != pl.DefaultClassifierModel {
		b.WriteString(warningStyle.Render("  (override)"))
	}
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Evaluator:   %s", cfg.EvaluatorModel))
	if cfg.EvaluatorModel != pl.DefaultEvaluatorModel {
		b.WriteString(warningStyle.Render("  (override)"))
	}
	return b.String()
}

func (m Model) renderPrompts() string {
	var b strings.Builder
	b.WriteString(sectionHeaderStyle.Render("PROMPTS"))
	b.WriteString("\n")
	for i, p := range m.prompts {
		age := relativeTime(p.UpdatedAt)
		b.WriteString(fmt.Sprintf("  %-20s %-4s  updated: %s", p.Name, p.Version, age))
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
	case "queued":
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

// formatTimingForMode formats per-stage timing based on layout mode.
//   - Wide: all 3 stages (parse, cls, eval)
//   - Standard: 2 stages (parse, eval) — classifier omitted
//   - Narrow: total duration only
func formatTimingForMode(run pl.PipelineRun, mode layout) string {
	if run.Status == "queued" {
		return mutedStyle.Render("(queued)")
	}

	if mode == layoutNarrow {
		total := run.ParseDuration + run.ClassifierDuration + run.EvaluatorDuration
		if total == 0 {
			return ""
		}
		return fmt.Sprintf("%.0fs", total.Seconds())
	}

	var parts []string
	if run.HasDigest && run.ParseDuration > 0 {
		parts = append(parts, fmt.Sprintf("%5.1fs parse", run.ParseDuration.Seconds()))
	}
	if mode == layoutWide {
		if run.HasClassifier {
			parts = append(parts, fmt.Sprintf("%5.1fs cls", run.ClassifierDuration.Seconds()))
		} else if run.Status == "error" && run.HasDigest {
			parts = append(parts, errorStyle.Render("  ✗ cls failed"))
		}
	}
	if run.HasEvaluator {
		parts = append(parts, fmt.Sprintf("%5.0fs eval", run.EvaluatorDuration.Seconds()))
	} else if run.Status == "error" && run.HasClassifier {
		parts = append(parts, errorStyle.Render(" ✗ eval failed"))
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

