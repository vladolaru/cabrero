package components

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// RenderHeader renders the persistent application header bar shown above every view.
// It is called by the root model, not by the dashboard view.
func RenderHeader(stats message.DashboardStats, width int) string {
	titleText := "  Cabrero"
	if stats.Version != "" {
		titleText += "  " + shared.MutedStyle.Render(stats.Version)
	}
	tagline := shared.MutedStyle.Render("  Shepherding AI pirate goats, one skill at a time")
	title := shared.HeaderStyle.Render(titleText) + "\n" + tagline

	var daemonStatus string
	if stats.DaemonRunning {
		daemonStatus = shared.SuccessStyle.Render("●") + fmt.Sprintf(" running (PID %d)", stats.DaemonPID)
	} else {
		daemonStatus = shared.ErrorStyle.Render("●") + " stopped"
	}

	var lastCapture string
	if stats.LastCaptureTime != nil {
		lastCapture = shared.MutedStyle.Render("Last capture:") + " " + shared.RelativeTime(*stats.LastCaptureTime)
	}

	hookPre := shared.Checkmark(stats.HookPreCompact)
	hookEnd := shared.Checkmark(stats.HookSessionEnd)
	hooks := shared.MutedStyle.Render("Hooks:") + fmt.Sprintf(" pre-compact %s  session-end %s", hookPre, hookEnd)

	debugIndicator := ""
	if stats.DebugMode {
		debugIndicator = "  " + shared.MutedStyle.Render("│  Debug:") + " " + shared.WarningStyle.Render("enabled")
	}

	if width >= 120 {
		// Wide: title on left, daemon/hooks on right.
		left := title
		rightLines := []string{
			shared.MutedStyle.Render("Daemon:") + " " + daemonStatus,
			lastCapture,
			hooks + debugIndicator,
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, left, "    ", strings.Join(rightLines, "\n"))
	}

	// Standard/narrow: stacked header.
	daemonLine := "  " + shared.MutedStyle.Render("Daemon:") + " " + daemonStatus
	if lastCapture != "" {
		daemonLine += "  " + shared.MutedStyle.Render("│") + "  " + lastCapture
	}
	return title + "\n" +
		daemonLine + "\n" +
		"  " + hooks + debugIndicator
}
