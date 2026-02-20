package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true)
	mutedStyle  = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"})
	accentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#6A1B9A", Dark: "#CE93D8"})
	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#66BB6A"})
	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#EF5350"})
	selectedStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})
)

// Type indicator characters.
const (
	indicatorProposal = "●"
)

// View renders the dashboard.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var b strings.Builder

	// Header.
	b.WriteString(m.renderHeader())
	b.WriteString("\n")

	// Separator.
	b.WriteString(strings.Repeat("─", m.width))
	b.WriteString("\n")

	// Content.
	if len(m.filtered) == 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  " + components.EmptyProposals()))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		b.WriteString(headerStyle.Render("  PENDING REVIEW"))
		b.WriteString("\n\n")
		b.WriteString(m.renderProposalList())
	}

	// Fill remaining space.
	content := b.String()
	lines := strings.Count(content, "\n")
	statusBarHeight := 1
	remaining := m.height - lines - statusBarHeight
	if remaining > 0 {
		content += strings.Repeat("\n", remaining)
	}

	// Filter bar or status bar.
	if m.filterActive {
		content += m.filterInput.View()
	} else {
		content += m.renderStatusBar()
	}

	return content
}

func (m Model) renderHeader() string {
	title := headerStyle.Render("  Cabrero Review")

	stats := fmt.Sprintf("  %d proposals awaiting review  ·  %d approved  ·  %d rejected",
		m.stats.PendingCount, m.stats.ApprovedCount, m.stats.RejectedCount)

	var daemonStatus string
	if m.stats.DaemonRunning {
		daemonStatus = successStyle.Render("●") + fmt.Sprintf(" running (PID %d)", m.stats.DaemonPID)
	} else {
		daemonStatus = errorStyle.Render("●") + " stopped"
	}

	var lastCapture string
	if m.stats.LastCaptureTime != nil {
		lastCapture = "Last capture: " + timeAgo(*m.stats.LastCaptureTime)
	}

	hookPre := checkMark(m.stats.HookPreCompact)
	hookEnd := checkMark(m.stats.HookSessionEnd)
	hooks := fmt.Sprintf("Hooks: pre-compact %s  session-end %s", hookPre, hookEnd)

	if m.width >= 120 {
		// Wide: stats on left, daemon/hooks on right.
		left := title + "\n" + stats
		right := fmt.Sprintf("Daemon: %s\n%s\n%s", daemonStatus, lastCapture, hooks)
		rightRendered := mutedStyle.Render(right)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", rightRendered)
	}

	// Standard/narrow: single-line header.
	return title + "\n" + mutedStyle.Render(stats) + "\n" +
		mutedStyle.Render(fmt.Sprintf("  Daemon: %s  │  %s  │  %s", daemonStatus, lastCapture, hooks))
}

func (m Model) renderProposalList() string {
	var b strings.Builder

	for i, p := range m.filtered {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		indicator := accentStyle.Render(indicatorProposal)
		typeName := padRight(p.Proposal.Type, 18)
		target := truncate(p.Proposal.Target, m.targetWidth())
		confidence := mutedStyle.Render(p.Proposal.Confidence)

		line := fmt.Sprintf("%s %s %s  %s  %s", prefix, indicator, typeName, target, confidence)

		if i == m.cursor {
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Sort indicator.
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  Sort: %s", m.sortOrder)))
	b.WriteString("\n")

	return b.String()
}

func (m Model) renderStatusBar() string {
	keys := m.keys
	return components.RenderStatusBar(keys.ShortHelp(), "", m.width)
}

func (m Model) targetWidth() int {
	if m.width >= 120 {
		return 40
	}
	if m.width >= 80 {
		return 30
	}
	return 20
}

func checkMark(ok bool) string {
	if ok {
		return successStyle.Render("✓")
	}
	return errorStyle.Render("✗")
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
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
		return padRight(s, maxLen)
	}
	return s[:maxLen-3] + "..."
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
