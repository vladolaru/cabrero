package shared

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

// Adaptive color pairs for light and dark terminals.
var (
	ColorSuccess = compat.AdaptiveColor{Light: lipgloss.Color("#2E7D32"), Dark: lipgloss.Color("#66BB6A")}
	ColorError   = compat.AdaptiveColor{Light: lipgloss.Color("#C62828"), Dark: lipgloss.Color("#EF5350")}
	ColorWarning = compat.AdaptiveColor{Light: lipgloss.Color("#E65100"), Dark: lipgloss.Color("#FFA726")}
	ColorAccent  = compat.AdaptiveColor{Light: lipgloss.Color("#6A1B9A"), Dark: lipgloss.Color("#CE93D8")}
	ColorMuted   = compat.AdaptiveColor{Light: lipgloss.Color("#757575"), Dark: lipgloss.Color("#9E9E9E")}
	ColorChat    = compat.AdaptiveColor{Light: lipgloss.Color("#00695C"), Dark: lipgloss.Color("#4DB6AC")}

	ColorFgBold      = compat.AdaptiveColor{Light: lipgloss.Color("#000000"), Dark: lipgloss.Color("#FFFFFF")}
	ColorBorder      = compat.AdaptiveColor{Light: lipgloss.Color("#BDBDBD"), Dark: lipgloss.Color("#616161")}
	ColorHighlightFg = compat.AdaptiveColor{Light: lipgloss.Color("#FFFFFF"), Dark: lipgloss.Color("#FFFFFF")}
	ColorHighlightBg = compat.AdaptiveColor{Light: lipgloss.Color("#6A1B9A"), Dark: lipgloss.Color("#9C27B0")}
)

// Common reusable lipgloss styles.
var (
	HeaderStyle   = lipgloss.NewStyle().Bold(true)
	MutedStyle    = lipgloss.NewStyle().Foreground(ColorMuted)
	SuccessStyle  = lipgloss.NewStyle().Foreground(ColorSuccess)
	ErrorStyle    = lipgloss.NewStyle().Foreground(ColorError)
	WarningStyle  = lipgloss.NewStyle().Foreground(ColorWarning)
	AccentStyle   = lipgloss.NewStyle().Foreground(ColorAccent)
	SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorFgBold)
)

// HighlightFg returns the foreground color string for search match highlighting.
// Selects the correct adaptive variant based on terminal background.
func HighlightFg() string {
	if compat.HasDarkBackground {
		return "#FFFFFF"
	}
	return "#FFFFFF"
}

// HighlightBg returns the background color string for search match highlighting.
// Selects the correct adaptive variant based on terminal background.
func HighlightBg() string {
	if compat.HasDarkBackground {
		return "#9C27B0"
	}
	return "#6A1B9A"
}
