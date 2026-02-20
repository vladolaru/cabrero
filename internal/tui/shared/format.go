package shared

import "strings"

// Truncate shortens s to maxLen characters, appending "..." if truncated.
// If maxLen is <= 0, returns empty. If maxLen <= 3, returns a raw slice.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// TruncatePad shortens s to maxLen (with "...") or pads with spaces to fill maxLen.
func TruncatePad(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) > maxLen {
		if maxLen <= 3 {
			return s[:maxLen]
		}
		return s[:maxLen-3] + "..."
	}
	return PadRight(s, maxLen)
}

// TruncateID shortens an ID string without ellipsis (raw slice).
func TruncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// PadRight pads s with spaces to the given width.
func PadRight(s string, width int) string {
	if width <= 0 || len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
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
