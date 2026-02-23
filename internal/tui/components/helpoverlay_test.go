package components

import (
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestRenderHelpOverlay_ContainsSectionTitles(t *testing.T) {
	sections := []shared.HelpSection{
		{
			Title: "Navigation",
			Entries: []shared.HelpEntry{
				{Key: "↑", Desc: "Move up"},
				{Key: "↓", Desc: "Move down"},
			},
		},
		{
			Title: "Actions",
			Entries: []shared.HelpEntry{
				{Key: "a", Desc: "Approve"},
			},
		},
	}

	output := RenderHelpOverlay(sections, 120, 40)

	for _, s := range sections {
		if !strings.Contains(output, s.Title) {
			t.Errorf("output missing section title %q", s.Title)
		}
	}
}

func TestRenderHelpOverlay_ContainsKeys(t *testing.T) {
	sections := []shared.HelpSection{
		{
			Title: "Test",
			Entries: []shared.HelpEntry{
				{Key: "ctrl+c", Desc: "Force quit"},
				{Key: "?", Desc: "Help"},
			},
		},
	}

	output := RenderHelpOverlay(sections, 120, 40)

	// Keys are rendered through lipgloss styles, but the raw text should appear.
	if !strings.Contains(output, "ctrl+c") {
		t.Error("output missing key \"ctrl+c\"")
	}
	if !strings.Contains(output, "?") {
		t.Error("output missing key \"?\"")
	}
}

func TestRenderHelpOverlay_Empty(t *testing.T) {
	// Empty sections should not panic and return minimal output.
	output := RenderHelpOverlay(nil, 120, 40)
	if output == "" {
		t.Error("expected non-empty output (at least top padding)")
	}
}
