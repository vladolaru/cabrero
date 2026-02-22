package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

var (
	headerStyle   = shared.HeaderStyle
	mutedStyle    = shared.MutedStyle
	accentStyle   = shared.AccentStyle
	warningStyle  = shared.WarningStyle
	successStyle  = shared.SuccessStyle
	errorStyle    = shared.ErrorStyle
	selectedStyle = shared.SelectedStyle
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
		b.WriteString(m.renderItemList())
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

	statsLine := fmt.Sprintf("  %d proposals awaiting review  ·  %d approved  ·  %d rejected",
		m.stats.PendingCount, m.stats.ApprovedCount, m.stats.RejectedCount)
	if m.stats.FitnessReportCount > 0 {
		statsLine += fmt.Sprintf("  ·  %d fitness reports", m.stats.FitnessReportCount)
	}

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

	debugIndicator := ""
	if m.stats.DebugMode {
		debugIndicator = "  │  Debug: " + warningStyle.Render("enabled")
	}

	if m.width >= 120 {
		// Wide: stats on left, daemon/hooks on right.
		left := title + "\n" + statsLine
		right := fmt.Sprintf("Daemon: %s\n%s\n%s%s", daemonStatus, lastCapture, hooks, debugIndicator)
		rightRendered := mutedStyle.Render(right)
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", rightRendered)
	}

	// Standard/narrow: stacked header.
	daemonLine := fmt.Sprintf("  Daemon: %s", daemonStatus)
	if lastCapture != "" {
		daemonLine += "  │  " + lastCapture
	}
	return title + "\n" + mutedStyle.Render(statsLine) + "\n" +
		mutedStyle.Render(daemonLine) + "\n" +
		mutedStyle.Render("  "+hooks+debugIndicator)
}

func (m Model) renderItemList() string {
	var b strings.Builder

	for i, item := range m.filtered {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		// Choose indicator style based on item type.
		var indicator string
		if item.IsProposal() {
			indicator = accentStyle.Render(indicatorProposal)
		} else {
			indicator = warningStyle.Render(indicatorFitness)
		}

		typeName := shared.PadRight(item.TypeName(), 18)
		target := shared.TruncatePad(item.Target(), m.targetWidth())
		confidence := mutedStyle.Render(item.Confidence())

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

	// Empty state: show only navigation keys that make sense.
	if len(m.filtered) == 0 {
		bindings := []key.Binding{keys.Sources, keys.Pipeline, keys.Help}
		return components.RenderStatusBar(bindings, "", m.width)
	}

	// Show different actions depending on the selected item type.
	item := m.SelectedItem()
	if item != nil && item.IsFitnessReport() {
		bindings := []key.Binding{
			keys.Up, keys.Down, keys.Open, keys.Sources, keys.Help,
		}
		return components.RenderStatusBar(bindings, "", m.width)
	}

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

