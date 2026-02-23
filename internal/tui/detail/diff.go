// Package detail implements the proposal detail view.
package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Diff rendering styles — reference shared color palette for consistency.
var (
	diffAddStyle  = lipgloss.NewStyle().Foreground(shared.ColorSuccess)
	diffDelStyle  = lipgloss.NewStyle().Foreground(shared.ColorError)
	diffHunkStyle = lipgloss.NewStyle().Foreground(shared.ColorMuted).Faint(true)
	diffLineNum   = lipgloss.NewStyle().Foreground(shared.ColorBorder)
	flaggedBox    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(shared.ColorWarning).
			Padding(0, 1)
)

// RenderDiff takes a unified diff string and returns a Lip Gloss styled string.
// For claude_review proposals, flaggedEntry is shown in a highlighted box.
// For skill_scaffold proposals, all lines are additions (green).
func RenderDiff(change *string, flaggedEntry *string, proposalType string, width int) string {
	// claude_review: no diff, just flagged entry.
	if proposalType == "claude_review" && flaggedEntry != nil {
		boxWidth := width - 4 // account for border
		if boxWidth < 20 {
			boxWidth = 20
		}
		return flaggedBox.Width(boxWidth).Render(*flaggedEntry)
	}

	if change == nil || *change == "" {
		return diffHunkStyle.Render("(no changes)")
	}

	diff := *change

	// skill_scaffold: treat entire content as additions.
	if proposalType == "skill_scaffold" {
		return renderScaffold(diff, width)
	}

	// If the content looks like a unified diff, render with syntax highlighting.
	// Otherwise treat as prose and word-wrap it.
	if looksLikeDiff(diff) {
		return renderUnifiedDiff(diff, width)
	}
	return renderProse(diff, width)
}

// looksLikeDiff returns true if the content appears to be a unified diff
// (contains +/- line markers or @@ hunk headers).
func looksLikeDiff(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "@@") {
			return true
		}
	}
	return false
}

// renderProse word-wraps a prose change description to fit within width.
func renderProse(text string, width int) string {
	wrapWidth := width - 4 // account for indent + prefix
	if wrapWidth < 20 {
		wrapWidth = 20
	}
	return lipgloss.NewStyle().Width(wrapWidth).Render(text)
}

func renderUnifiedDiff(diff string, width int) string {
	lines := strings.Split(diff, "\n")
	var b strings.Builder
	lineNo := 0

	for _, line := range lines {
		if line == "" {
			b.WriteRune('\n')
			continue
		}

		switch {
		case strings.HasPrefix(line, "@@"):
			b.WriteString(diffHunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			lineNo++
			num := fmt.Sprintf("%3d ", lineNo)
			b.WriteString(diffLineNum.Render(num))
			b.WriteString(diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(diffLineNum.Render("    "))
			b.WriteString(diffDelStyle.Render(line))
		default:
			lineNo++
			num := fmt.Sprintf("%3d ", lineNo)
			b.WriteString(diffLineNum.Render(num))
			b.WriteString(line)
		}
		b.WriteRune('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderScaffold(content string, width int) string {
	lines := strings.Split(content, "\n")
	var b strings.Builder

	for i, line := range lines {
		num := fmt.Sprintf("%3d ", i+1)
		b.WriteString(diffLineNum.Render(num))
		b.WriteString(diffAddStyle.Render("+ " + line))
		b.WriteRune('\n')
	}

	return strings.TrimRight(b.String(), "\n")
}
