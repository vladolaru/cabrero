package cmd

import (
	"fmt"
	"time"
)

// formatAge returns a human-readable relative time string.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}

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
