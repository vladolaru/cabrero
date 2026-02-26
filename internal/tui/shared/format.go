package shared

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Checkmark renders a ✓ (success) or ✗ (error) with appropriate color.
func Checkmark(ok bool) string {
	if ok {
		return SuccessStyle.Render("✓")
	}
	return ErrorStyle.Render("✗")
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

// FillToBottom pads content with newlines so the total height is
// (totalHeight - reservedLines). Use reservedLines=1 for a status bar.
// Returns content unchanged if it already meets or exceeds the target.
func FillToBottom(content string, totalHeight, reservedLines int) string {
	lines := strings.Count(content, "\n")
	remaining := totalHeight - lines - reservedLines
	if remaining > 0 {
		return content + strings.Repeat("\n", remaining)
	}
	return content
}

// RenderSubHeader renders the standard two-line view sub-header:
// a bold title on the first line and a muted stats string on the second.
func RenderSubHeader(title, stats string) string {
	return HeaderStyle.Render(title) + "\n" + MutedStyle.Render(stats)
}

// sectionSeparatorLen is the character width of the separator under section titles.
// Matches the visual width of the longest section title ("PROPOSED CHANGE" = 15) plus indent.
const sectionSeparatorLen = 17

// RenderSectionHeader renders a bold accent section title with a separator line below it.
// Both title and separator are indented by two spaces.
func RenderSectionHeader(title string) string {
	return AccentBoldStyle.Render("  "+title) + "\n" + "  " + strings.Repeat("─", sectionSeparatorLen)
}
