// Package cli provides colored output helpers for CLI commands.
// Colors mirror the TUI palette (internal/tui/shared/styles.go) but this
// package has no dependency on TUI internals.
package cli

import (
	"os"

	"charm.land/lipgloss/v2"
	"github.com/muesli/termenv"
)

var (
	styleSuccess lipgloss.Style
	styleError   lipgloss.Style
	styleWarning lipgloss.Style
	styleAccent  lipgloss.Style
	styleMuted   lipgloss.Style
	styleBold    = lipgloss.NewStyle().Bold(true)
)

func init() {
	out := termenv.NewOutput(os.Stdout)
	ld := lipgloss.LightDark(out.HasDarkBackground())

	styleSuccess = lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#2E7D32"), lipgloss.Color("#66BB6A")))
	styleError = lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#C62828"), lipgloss.Color("#EF5350")))
	styleWarning = lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#E65100"), lipgloss.Color("#FFA726")))
	styleAccent = lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#CE93D8")))
	styleMuted = lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#757575"), lipgloss.Color("#9E9E9E")))
}

// Success renders text in green (pass/done).
func Success(s string) string { return styleSuccess.Render(s) }

// Error renders text in red (failure).
func Error(s string) string { return styleError.Render(s) }

// Warn renders text in orange (warning/attention).
func Warn(s string) string { return styleWarning.Render(s) }

// Accent renders text in purple (action/highlight).
func Accent(s string) string { return styleAccent.Render(s) }

// Muted renders text in gray (skipped/secondary).
func Muted(s string) string { return styleMuted.Render(s) }

// Bold renders text in bold.
func Bold(s string) string { return styleBold.Render(s) }
