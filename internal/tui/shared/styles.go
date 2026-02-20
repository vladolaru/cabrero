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

	ColorFg     = lipgloss.AdaptiveColor{Light: "#212121", Dark: "#E0E0E0"}
	ColorFgBold = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}
	ColorBg     = lipgloss.AdaptiveColor{Light: "#FAFAFA", Dark: "#1E1E1E"}
	ColorBorder = lipgloss.AdaptiveColor{Light: "#BDBDBD", Dark: "#616161"}
)

// Styles holds all pre-computed Lip Gloss styles for the TUI.
type Styles struct {
	// Borders
	ActiveBorder   lipgloss.Style
	InactiveBorder lipgloss.Style

	// List items
	SelectedItem lipgloss.Style
	NormalItem   lipgloss.Style

	// Text
	SectionHeader lipgloss.Style
	MutedText     lipgloss.Style
	AccentText    lipgloss.Style
	BoldText      lipgloss.Style
	ErrorText     lipgloss.Style
	SuccessText   lipgloss.Style
	WarningText   lipgloss.Style
	ChatText      lipgloss.Style

	// Diff
	DiffAdd  lipgloss.Style
	DiffDel  lipgloss.Style
	DiffHunk lipgloss.Style

	// Status bar
	StatusBar     lipgloss.Style
	StatusBarKey  lipgloss.Style
	StatusBarText lipgloss.Style

	// Indicators
	TypeIndicator lipgloss.Style
	Spinner       lipgloss.Style

	// Citation
	CitationBox lipgloss.Style
}

// DefaultStyles returns styles using the adaptive color palette.
func DefaultStyles() Styles {
	return Styles{
		ActiveBorder: lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorAccent),
		InactiveBorder: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(ColorMuted),

		SelectedItem: lipgloss.NewStyle().
			Foreground(ColorFgBold).
			Background(ColorAccent).
			Bold(true),
		NormalItem: lipgloss.NewStyle().
			Foreground(ColorFg),

		SectionHeader: lipgloss.NewStyle().
			Foreground(ColorFg).
			Bold(true),
		MutedText: lipgloss.NewStyle().
			Foreground(ColorMuted),
		AccentText: lipgloss.NewStyle().
			Foreground(ColorAccent),
		BoldText: lipgloss.NewStyle().
			Foreground(ColorFgBold).
			Bold(true),
		ErrorText: lipgloss.NewStyle().
			Foreground(ColorError),
		SuccessText: lipgloss.NewStyle().
			Foreground(ColorSuccess),
		WarningText: lipgloss.NewStyle().
			Foreground(ColorWarning),
		ChatText: lipgloss.NewStyle().
			Foreground(ColorChat),

		DiffAdd: lipgloss.NewStyle().
			Foreground(ColorSuccess),
		DiffDel: lipgloss.NewStyle().
			Foreground(ColorError),
		DiffHunk: lipgloss.NewStyle().
			Foreground(ColorMuted).
			Faint(true),

		StatusBar: lipgloss.NewStyle().
			Foreground(ColorFgBold).
			Background(ColorBorder),
		StatusBarKey: lipgloss.NewStyle().
			Foreground(ColorAccent).
			Background(ColorBorder).
			Bold(true),
		StatusBarText: lipgloss.NewStyle().
			Foreground(ColorFg).
			Background(ColorBorder),

		TypeIndicator: lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true),
		Spinner: lipgloss.NewStyle().
			Foreground(ColorAccent),

		CitationBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1),
	}
}

// ThemeFromConfig creates styles. Currently always returns DefaultStyles
// since the adaptive palette handles light/dark automatically. The theme
// config field ("auto"/"dark"/"light") is reserved for future forced overrides.
func ThemeFromConfig(_ *Config) Styles {
	return DefaultStyles()
}
