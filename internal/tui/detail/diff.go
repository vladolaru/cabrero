// Package detail implements the proposal detail view.
package detail

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Diff rendering styles — use adaptive colors for terminal compatibility.
var (
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2E7D32", Dark: "#66BB6A"})
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#C62828", Dark: "#EF5350"})
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#757575", Dark: "#9E9E9E"}).Faint(true)
	diffLineNum   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#9E9E9E", Dark: "#616161"})
	flaggedBox    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#E65100", Dark: "#FFA726"}).
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

	return renderUnifiedDiff(diff, width)
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
