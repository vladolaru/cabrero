package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/components"
	"github.com/vladolaru/cabrero/internal/tui/message"
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

	// Column headers and content.
	if len(m.filtered) == 0 {
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render("  " + components.EmptyProposals()))
		b.WriteString("\n")
	} else {
		b.WriteString(m.renderColumnHeaders())
		b.WriteString("\n")
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

// RenderHeader renders the persistent header bar (title, version, daemon status, hooks).
// It is called by the root model to render above every child view.
func RenderHeader(stats message.DashboardStats, width int) string {
	titleText := "  Cabrero"
	if stats.Version != "" {
		titleText += "  " + mutedStyle.Render(stats.Version)
	}
	tagline := mutedStyle.Render("  Shepherding AI pirate goats, one skill at a time")
	title := headerStyle.Render(titleText) + "\n" + tagline

	var daemonStatus string
	if stats.DaemonRunning {
		daemonStatus = successStyle.Render("●") + fmt.Sprintf(" running (PID %d)", stats.DaemonPID)
	} else {
		daemonStatus = errorStyle.Render("●") + " stopped"
	}

	var lastCapture string
	if stats.LastCaptureTime != nil {
		lastCapture = mutedStyle.Render("Last capture:") + " " + timeAgo(*stats.LastCaptureTime)
	}

	hookPre := checkMark(stats.HookPreCompact)
	hookEnd := checkMark(stats.HookSessionEnd)
	hooks := mutedStyle.Render("Hooks:") + fmt.Sprintf(" pre-compact %s  session-end %s", hookPre, hookEnd)

	debugIndicator := ""
	if stats.DebugMode {
		debugIndicator = "  " + mutedStyle.Render("│  Debug:") + " " + warningStyle.Render("enabled")
	}

	if width >= 120 {
		// Wide: title on left, daemon/hooks on right.
		left := title
		rightLines := []string{
			mutedStyle.Render("Daemon:") + " " + daemonStatus,
			lastCapture,
			hooks + debugIndicator,
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", strings.Join(rightLines, "\n"))
	}

	// Standard/narrow: stacked header.
	daemonLine := "  " + mutedStyle.Render("Daemon:") + " " + daemonStatus
	if lastCapture != "" {
		daemonLine += "  " + mutedStyle.Render("│") + "  " + lastCapture
	}
	return title + "\n" +
		daemonLine + "\n" +
		"  " + hooks + debugIndicator
}

// SubHeader returns the view title and stats line for the dashboard.
func (m Model) SubHeader() string {
	title := headerStyle.Render("  Proposals")

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

	return title + "\n" + mutedStyle.Render(statsLine)
}

func (m Model) renderColumnHeaders() string {
	cols := m.columnLayout()

	// Row: prefix(2) + " " + indicator(1) + " " + type(18) + "  " + target + "  " + confidence
	// TYPE aligns with the bullet indicator at position 3 (prefix + space).
	header := shared.PadRight("   TYPE", cols.typeWidth+3) +
		"  " + shared.PadRight("TARGET", cols.targetWidth) +
		"  " + "CONFIDENCE"

	return mutedStyle.Render(header)
}

func (m Model) renderItemList() string {
	var b strings.Builder
	cols := m.columnLayout()

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

		typeName := shared.PadRight(item.TypeName(), cols.typeWidth)
		target := shared.TruncatePad(shared.ShortenHome(item.Target()), cols.targetWidth)
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

// Column layout.

const (
	colType       = 18 // longest type: "skill_improvement" = 17 + 1
	colConfidence = 12 // longest: "100% health" = 11 + 1
)

type columnSpec struct {
	typeWidth   int
	targetWidth int
}

func (m Model) columnLayout() columnSpec {
	// Row: prefix(2) + " " + indicator(1) + " " + type + "  " + target + "  " + confidence
	// Fixed overhead = 5 + typeWidth + 2 + 2 + confidenceWidth
	overhead := 5 + colType + 2 + 2 + colConfidence
	targetWidth := m.width - overhead
	if targetWidth < 15 {
		targetWidth = 15
	}
	return columnSpec{typeWidth: colType, targetWidth: targetWidth}
}

func checkMark(ok bool) string {
	if ok {
		return successStyle.Render("✓")
	}
	return errorStyle.Render("✗")
}

