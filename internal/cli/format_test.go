package cli

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		t    time.Time
		want string
	}{
		{"zero time", time.Time{}, "unknown"},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-48 * time.Hour), "2d ago"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RelativeTime(tt.t); got != tt.want {
				t.Errorf("RelativeTime() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestShortenHome(t *testing.T) {
	if homeDir == "" {
		t.Skip("home dir not available")
	}
	full := homeDir + "/.claude/SKILL.md"
	got := ShortenHome(full)
	want := "~/.claude/SKILL.md"
	if got != want {
		t.Errorf("ShortenHome(%q) = %q, want %q", full, got, want)
	}
	other := "/etc/hosts"
	if got := ShortenHome(other); got != other {
		t.Errorf("ShortenHome(%q) = %q, want unchanged", other, got)
	}
}
