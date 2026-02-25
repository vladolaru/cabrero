// Package cli provides colored output helpers for CLI commands.
// Colors mirror the TUI palette (internal/tui/shared/styles.go) but this
// package has no dependency on TUI internals.
package cli

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Adaptive color pairs for light and dark terminals.
var (
	colorSuccess = compat.AdaptiveColor{Light: lipgloss.Color("#2E7D32"), Dark: lipgloss.Color("#66BB6A")}
	colorError   = compat.AdaptiveColor{Light: lipgloss.Color("#C62828"), Dark: lipgloss.Color("#EF5350")}
	colorWarning = compat.AdaptiveColor{Light: lipgloss.Color("#E65100"), Dark: lipgloss.Color("#FFA726")}
	colorAccent  = compat.AdaptiveColor{Light: lipgloss.Color("#6A1B9A"), Dark: lipgloss.Color("#CE93D8")}
	colorMuted   = compat.AdaptiveColor{Light: lipgloss.Color("#757575"), Dark: lipgloss.Color("#9E9E9E")}
)

var (
	styleSuccess = lipgloss.NewStyle().Foreground(colorSuccess)
	styleError   = lipgloss.NewStyle().Foreground(colorError)
	styleWarning = lipgloss.NewStyle().Foreground(colorWarning)
	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent)
	styleMuted   = lipgloss.NewStyle().Foreground(colorMuted)
	styleBold    = lipgloss.NewStyle().Bold(true)
)

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
