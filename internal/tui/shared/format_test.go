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

func TestRenderSectionHeader(t *testing.T) {
	result := RenderSectionHeader("ASSESSMENT")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "ASSESSMENT") {
		t.Error("section header should contain title")
	}
	if !strings.Contains(stripped, "─") {
		t.Error("section header should contain separator")
	}
	if strings.Count(result, "\n") != 1 {
		t.Errorf("section header should have 1 newline, got %d", strings.Count(result, "\n"))
	}
}

func TestFillToBottom_AddsNewlines(t *testing.T) {
	content := "line1\nline2\nline3"
	// content has 2 newlines → 3 lines rendered. Height=10, reserved=1 (status bar).
	// remaining = 10 - 2 - 1 = 7 → append 7 newlines.
	result := FillToBottom(content, 10, 1)
	newlines := strings.Count(result, "\n")
	if newlines != 9 { // 2 existing + 7 added
		t.Errorf("FillToBottom newlines = %d, want 9", newlines)
	}
}

func TestFillToBottom_NoOpWhenAlreadyFull(t *testing.T) {
	content := strings.Repeat("line\n", 10)
	result := FillToBottom(content, 10, 0)
	if result != content {
		t.Error("FillToBottom should not modify content that already fills height")
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
