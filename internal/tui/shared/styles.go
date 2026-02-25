package shared

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// IsDark reports whether the terminal has a dark background.
// Set by InitStyles; updated by ReinitStyles when BackgroundColorMsg arrives.
var IsDark bool

// Color palette — concrete color.Color values set by InitStyles/ReinitStyles.
var (
	ColorSuccess     color.Color
	ColorError       color.Color
	ColorWarning     color.Color
	ColorAccent      color.Color
	ColorMuted       color.Color
	ColorChat        color.Color
	ColorFgBold      color.Color
	ColorBorder      color.Color
	ColorHighlightFg color.Color
	ColorHighlightBg color.Color
)

// Reusable lipgloss styles — rebuilt by InitStyles/ReinitStyles.
var (
	HeaderStyle     lipgloss.Style
	MutedStyle      lipgloss.Style
	SuccessStyle    lipgloss.Style
	ErrorStyle      lipgloss.Style
	WarningStyle    lipgloss.Style
	AccentStyle     lipgloss.Style
	SelectedStyle   lipgloss.Style
	AccentBoldStyle lipgloss.Style
)

// InitStyles sets all color and style vars for the given background.
// Call once from tui.go before tea.NewProgram. Dark (true) is the assumed
// default; BackgroundColorMsg will update to the correct value within the
// first frames.
func InitStyles(isDark bool) {
	IsDark = isDark
	ld := lipgloss.LightDark(isDark)

	ColorSuccess = ld(lipgloss.Color("#2E7D32"), lipgloss.Color("#66BB6A"))
	ColorError = ld(lipgloss.Color("#C62828"), lipgloss.Color("#EF5350"))
	ColorWarning = ld(lipgloss.Color("#E65100"), lipgloss.Color("#FFA726"))
	ColorAccent = ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#CE93D8"))
	ColorMuted = ld(lipgloss.Color("#757575"), lipgloss.Color("#9E9E9E"))
	ColorChat = ld(lipgloss.Color("#00695C"), lipgloss.Color("#4DB6AC"))
	ColorFgBold = ld(lipgloss.Color("#000000"), lipgloss.Color("#FFFFFF"))
	ColorBorder = ld(lipgloss.Color("#BDBDBD"), lipgloss.Color("#616161"))
	ColorHighlightFg = ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#FFFFFF"))
	ColorHighlightBg = ld(lipgloss.Color("#6A1B9A"), lipgloss.Color("#9C27B0"))

	HeaderStyle = lipgloss.NewStyle().Bold(true)
	MutedStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	ErrorStyle = lipgloss.NewStyle().Foreground(ColorError)
	WarningStyle = lipgloss.NewStyle().Foreground(ColorWarning)
	AccentStyle = lipgloss.NewStyle().Foreground(ColorAccent)
	SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorFgBold)
	AccentBoldStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
}

// ReinitStyles rebuilds all styles for the updated background. Call from
// appModel.Update() when tea.BackgroundColorMsg arrives.
func ReinitStyles(isDark bool) {
	InitStyles(isDark)
}

// HighlightFg returns the foreground hex for search match highlighting.
func HighlightFg() string {
	return "#FFFFFF" // same on both backgrounds
}

// HighlightBg returns the background hex for search match highlighting.
func HighlightBg() string {
	if IsDark {
		return "#9C27B0"
	}
	return "#6A1B9A"
}
