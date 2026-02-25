package cli

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// homeDir is resolved once at init so ShortenHome never calls os.UserHomeDir on the hot path.
var homeDir string

func init() {
	homeDir, _ = os.UserHomeDir()
}

// RelativeTime formats t as a human-readable relative age string.
// Returns "unknown" for a zero time; "just now" for durations under 1 minute.
func RelativeTime(t time.Time) string {
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

// ShortenHome replaces the current user's home directory prefix with "~".
// Returns path unchanged if home cannot be determined or path is not under home.
func ShortenHome(path string) string {
	if homeDir != "" && strings.HasPrefix(path, homeDir) {
		return "~" + path[len(homeDir):]
	}
	return path
}
