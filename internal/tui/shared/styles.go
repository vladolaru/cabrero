package shared

import "github.com/charmbracelet/lipgloss"

// Adaptive color pairs for light and dark terminals.
var (
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#66BB6A"}
	ColorError   = lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#EF5350"}
	ColorWarning = lipgloss.AdaptiveColor{Light: "#E65100", Dark: "#FFA726"}
	ColorAccent  = lipgloss.AdaptiveColor{Light: "#6A1B9A", Dark: "#CE93D8"}
	ColorMuted   = lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"}
	ColorChat    = lipgloss.AdaptiveColor{Light: "#00695C", Dark: "#4DB6AC"}

	ColorFgBold      = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}
	ColorBorder      = lipgloss.AdaptiveColor{Light: "#BDBDBD", Dark: "#616161"}
	ColorHighlightFg = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
	ColorHighlightBg = lipgloss.AdaptiveColor{Light: "#6A1B9A", Dark: "#9C27B0"}
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
	if lipgloss.HasDarkBackground() {
		return ColorHighlightFg.Dark
	}
	return ColorHighlightFg.Light
}

// HighlightBg returns the background color string for search match highlighting.
// Selects the correct adaptive variant based on terminal background.
func HighlightBg() string {
	if lipgloss.HasDarkBackground() {
		return ColorHighlightBg.Dark
	}
	return ColorHighlightBg.Light
}

