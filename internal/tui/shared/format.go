package shared

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// homeDir is resolved once at init for ShortenHome.
var homeDir string

func init() {
	homeDir, _ = os.UserHomeDir()
}

// ShortenHome replaces the current user's home directory prefix with "~".
func ShortenHome(path string) string {
	if homeDir != "" && strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}

// Truncate shortens s to maxLen runes, appending "..." if truncated.
// If maxLen is <= 0, returns empty. If maxLen <= 3, returns a raw slice.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-3]) + "..."
}

// TruncatePad shortens s to maxLen runes (with "...") or pads with spaces to fill maxLen.
func TruncatePad(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) > maxLen {
		if maxLen <= 3 {
			return string(runes[:maxLen])
		}
		return string(runes[:maxLen-3]) + "..."
	}
	return PadRight(s, maxLen)
}

// TruncateID shortens an ID string without ellipsis (raw slice).
func TruncateID(id string, maxLen int) string {
	runes := []rune(id)
	if len(runes) <= maxLen {
		return id
	}
	return string(runes[:maxLen])
}

// PadRight pads s with spaces to the given width (measured in runes).
func PadRight(s string, width int) string {
	runeLen := len([]rune(s))
	if width <= 0 || runeLen >= width {
		return s
	}
	return s + strings.Repeat(" ", width-runeLen)
}

// IndentBlock indents every line of s by the given number of spaces.
func IndentBlock(s string, spaces int) string {
	indent := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// WrapIndent word-wraps s to fit within the given total width, then indents
// each line by the given number of spaces. The text portion is wrapped to
// (width - indent) characters so that the final output fits within width.
func WrapIndent(s string, width, indent int) string {
	textWidth := width - indent
	if textWidth < 10 {
		textWidth = 10
	}
	wrapped := lipgloss.NewStyle().Width(textWidth).Render(s)
	return IndentBlock(wrapped, indent)
}

// RelativeTime formats t as a human-readable relative age string.
// Returns "just now" for durations under 1 minute.
func RelativeTime(t time.Time) string {
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

// WrapHangingIndent word-wraps s to fit within (width - indent) characters.
// The first line is returned without indent; continuation lines are indented.
func WrapHangingIndent(s string, width, indent int) string {
	textWidth := width - indent
	if textWidth < 10 {
		textWidth = 10
	}
	wrapped := lipgloss.NewStyle().Width(textWidth).Render(s)
	pad := strings.Repeat(" ", indent)
	lines := strings.Split(wrapped, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + lines[i]
	}
	return strings.Join(lines, "\n")
}
