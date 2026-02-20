package components

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// barFilled is the character used for the filled portion of the bar.
const barFilled = '█'

// barEmpty is the character used for the empty portion of the bar.
const barEmpty = '░'

// assessRow holds the data needed to render one assessment row.
type assessRow struct {
	label    string
	count    int
	percent  float64
	category string
}

// categoryColor returns the lipgloss color for a given assessment category.
func categoryColor(category string) lipgloss.TerminalColor {
	switch category {
	case "followed":
		return shared.ColorSuccess
	case "worked_around":
		return shared.ColorWarning
	case "confused":
		return shared.ColorError
	default:
		return shared.ColorMuted
	}
}

// RenderAssessBar renders a horizontal fitness bar.
// percent is 0-100, width is the total character width available for the bar.
// category determines the filled portion color (green/yellow/red).
func RenderAssessBar(percent float64, width int, category string) string {
	if width <= 0 {
		return ""
	}

	// Clamp percent to [0, 100].
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}

	filled := int(math.Round(float64(width) * percent / 100))
	if filled > width {
		filled = width
	}
	empty := width - filled

	color := categoryColor(category)
	filledStyle := lipgloss.NewStyle().Foreground(color)

	bar := filledStyle.Render(strings.Repeat(string(barFilled), filled)) +
		strings.Repeat(string(barEmpty), empty)

	return bar
}

// RenderAssessment renders the full three-row assessment with labels and bars.
// Each row follows the layout:
//
//	Label ..... N sessions  ████████░░░░░░░░░░  NN%
func RenderAssessment(assessment fitness.Assessment, width int) string {
	rows := []assessRow{
		{label: "Followed correctly", count: assessment.Followed.Count, percent: assessment.Followed.Percent, category: "followed"},
		{label: "Worked around", count: assessment.WorkedAround.Count, percent: assessment.WorkedAround.Percent, category: "worked_around"},
		{label: "Confused", count: assessment.Confused.Count, percent: assessment.Confused.Percent, category: "confused"},
	}

	// Find the longest label for alignment.
	maxLabel := 0
	for _, r := range rows {
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}

	var lines []string
	for _, r := range rows {
		lines = append(lines, renderAssessRow(r, maxLabel, width))
	}

	return strings.Join(lines, "\n")
}

// renderAssessRow renders a single assessment row.
func renderAssessRow(r assessRow, labelWidth, totalWidth int) string {
	// Format: "  Label ..... N sessions  ████████░░░░░░░░  NN%"
	indent := "  "

	// Right side: " NN%" — always 4-5 chars.
	pctStr := fmt.Sprintf("%3.0f%%", r.percent)

	// Count section: " N sessions"
	unit := "sessions"
	if r.count == 1 {
		unit = "session"
	}
	countStr := fmt.Sprintf("%d %s", r.count, unit)

	// Calculate how much space the bar gets.
	// Layout: indent + label + dots + " " + count + "  " + bar + "  " + pct
	//   indent = 2
	//   label = labelWidth
	//   dots = variable (min 1)
	//   " " = 1
	//   count = len(countStr)
	//   "  " = 2
	//   bar = variable
	//   "  " = 2
	//   pct = len(pctStr)
	fixedWidth := len(indent) + labelWidth + 1 + len(countStr) + 2 + 2 + len(pctStr)

	// Give a reasonable portion to dots and bar.
	// We want dots to fill between label and count, and bar to take remaining space.
	// Let's allocate: bar gets about 40% of total width (min 10).
	barWidth := totalWidth * 40 / 100
	if barWidth < 10 {
		barWidth = 10
	}

	// Dots fill the gap between the label and count.
	dotsAvail := totalWidth - fixedWidth - barWidth
	if dotsAvail < 1 {
		dotsAvail = 1
	}

	// Build the label with dot leader.
	label := r.label
	dotPad := labelWidth - len(label) + dotsAvail
	if dotPad < 1 {
		dotPad = 1
	}
	dots := " " + strings.Repeat(".", dotPad-1)
	if dotPad <= 1 {
		dots = " "
	}

	mutedStyle := lipgloss.NewStyle().Foreground(shared.ColorMuted)

	bar := RenderAssessBar(r.percent, barWidth, r.category)

	return indent + label + mutedStyle.Render(dots) + " " + countStr + "  " + bar + "  " + pctStr
}
