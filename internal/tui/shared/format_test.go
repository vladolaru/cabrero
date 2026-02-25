package shared

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestRelativeTime(t *testing.T) {
	now := time.Now()
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-2 * 24 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		got := RelativeTime(c.t)
		if got != c.want {
			t.Errorf("RelativeTime(%v) = %q, want %q", time.Since(c.t).Round(time.Second), got, c.want)
		}
	}
}

func TestRenderSubHeader(t *testing.T) {
	result := RenderSubHeader("  Proposals", "  3 awaiting review")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "Proposals") {
		t.Error("SubHeader should contain title")
	}
	if !strings.Contains(stripped, "3 awaiting review") {
		t.Error("SubHeader should contain stats")
	}
	// Should be exactly two lines.
	if strings.Count(result, "\n") != 1 {
		t.Errorf("SubHeader should have exactly 1 newline, got %d", strings.Count(result, "\n"))
	}
}

func TestCheckmark(t *testing.T) {
	ok := Checkmark(true)
	if ok == "" {
		t.Error("Checkmark(true) should return non-empty string")
	}
	notOk := Checkmark(false)
	if notOk == "" {
		t.Error("Checkmark(false) should return non-empty string")
	}
	if ok == notOk {
		t.Error("Checkmark(true) and Checkmark(false) should differ")
	}
}
