package components

// sparkChars are Unicode block elements for sparkline rendering, ordered
// from lowest to highest.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// RenderSparkline renders a sparkline string from integer data.
// Each value maps to a Unicode block character proportional to the max value.
// Output is truncated to width characters. Returns "" for empty data.
func RenderSparkline(data []int, width int) string {
	if len(data) == 0 {
		return ""
	}

	// Find max for scaling.
	maxVal := 0
	for _, v := range data {
		if v > maxVal {
			maxVal = v
		}
	}

	// Truncate to width.
	display := data
	if len(display) > width {
		display = display[:width]
	}

	result := make([]rune, len(display))
	for i, v := range display {
		if maxVal == 0 {
			result[i] = sparkChars[0]
			continue
		}
		// Scale to 0..7 range.
		idx := v * (len(sparkChars) - 1) / maxVal
		result[i] = sparkChars[idx]
	}
	return string(result)
}
