package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestRenderAssessBar_Full(t *testing.T) {
	got := ansi.Strip(RenderAssessBar(100, 20, "followed"))

	filled := strings.Count(got, string(barFilled))
	empty := strings.Count(got, string(barEmpty))

	if filled != 20 {
		t.Errorf("expected 20 filled chars, got %d", filled)
	}
	if empty != 0 {
		t.Errorf("expected 0 empty chars, got %d", empty)
	}
}

func TestRenderAssessBar_Empty(t *testing.T) {
	got := ansi.Strip(RenderAssessBar(0, 20, "confused"))

	filled := strings.Count(got, string(barFilled))
	empty := strings.Count(got, string(barEmpty))

	if filled != 0 {
		t.Errorf("expected 0 filled chars, got %d", filled)
	}
	if empty != 20 {
		t.Errorf("expected 20 empty chars, got %d", empty)
	}
}

func TestRenderAssessBar_Half(t *testing.T) {
	got := ansi.Strip(RenderAssessBar(50, 20, "worked_around"))

	filled := strings.Count(got, string(barFilled))
	empty := strings.Count(got, string(barEmpty))

	if filled != 10 {
		t.Errorf("expected 10 filled chars, got %d", filled)
	}
	if empty != 10 {
		t.Errorf("expected 10 empty chars, got %d", empty)
	}
}

func TestRenderBar_FilledEmpty(t *testing.T) {
	// At 0%: all empty.
	bar := RenderBar(0, 10, shared.ColorSuccess)
	stripped := ansi.Strip(bar)
	if strings.ContainsRune(stripped, barFilled) {
		t.Error("0% bar should have no filled chars")
	}
	if len([]rune(stripped)) != 10 {
		t.Errorf("bar width = %d, want 10", len([]rune(stripped)))
	}

	// At 100%: all filled.
	bar = RenderBar(100, 10, shared.ColorSuccess)
	stripped = ansi.Strip(bar)
	if strings.ContainsRune(stripped, barEmpty) {
		t.Error("100% bar should have no empty chars")
	}

	// At 50%: half filled.
	bar = RenderBar(50, 10, shared.ColorSuccess)
	stripped = ansi.Strip(bar)
	filled := strings.Count(stripped, string(barFilled))
	empty := strings.Count(stripped, string(barEmpty))
	if filled != 5 || empty != 5 {
		t.Errorf("50%% bar: filled=%d empty=%d, want 5 each", filled, empty)
	}
}

func TestRenderAssessment_ThreeBuckets(t *testing.T) {
	assessment := fitness.Assessment{
		Followed: fitness.BucketStat{
			Count:   5,
			Percent: 50,
		},
		WorkedAround: fitness.BucketStat{
			Count:   3,
			Percent: 30,
		},
		Confused: fitness.BucketStat{
			Count:   2,
			Percent: 20,
		},
	}

	got := ansi.Strip(RenderAssessment(assessment, 80))
	lines := strings.Split(got, "\n")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), got)
	}

	// Verify each row contains the expected label and count.
	expectations := []struct {
		label string
		count string
		pct   string
	}{
		{label: "Followed correctly", count: "5 sessions", pct: "50%"},
		{label: "Worked around", count: "3 sessions", pct: "30%"},
		{label: "Confused", count: "2 sessions", pct: "20%"},
	}

	for i, exp := range expectations {
		line := lines[i]
		if !strings.Contains(line, exp.label) {
			t.Errorf("line %d: expected label %q, got %q", i, exp.label, line)
		}
		if !strings.Contains(line, exp.count) {
			t.Errorf("line %d: expected count %q, got %q", i, exp.count, line)
		}
		if !strings.Contains(line, exp.pct) {
			t.Errorf("line %d: expected percent %q, got %q", i, exp.pct, line)
		}
		// Verify bar characters are present.
		if !strings.ContainsRune(line, barFilled) && !strings.ContainsRune(line, barEmpty) {
			t.Errorf("line %d: expected bar characters, got %q", i, line)
		}
	}
}
