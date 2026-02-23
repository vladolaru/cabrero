package components

import (
	"strings"
	"testing"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

func TestRenderHelpOverlay_ContainsSectionTitles(t *testing.T) {
	hc := shared.HelpContent{
		Title:       "Test Help",
		Description: "A test view.",
		Sections: []shared.HelpSection{
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
		},
	}

	output := RenderHelpOverlay(hc, 120, 40)

	for _, s := range hc.Sections {
		if !strings.Contains(output, s.Title) {
			t.Errorf("output missing section title %q", s.Title)
		}
	}
}

func TestRenderHelpOverlay_ContainsKeys(t *testing.T) {
	hc := shared.HelpContent{
		Title:       "Test Help",
		Description: "A test view.",
		Sections: []shared.HelpSection{
			{
				Title: "Test",
				Entries: []shared.HelpEntry{
					{Key: "ctrl+c", Desc: "Force quit"},
					{Key: "?", Desc: "Help"},
				},
			},
		},
	}

	output := RenderHelpOverlay(hc, 120, 40)

	// Keys are rendered through lipgloss styles, but the raw text should appear.
	if !strings.Contains(output, "ctrl+c") {
		t.Error("output missing key \"ctrl+c\"")
	}
	if !strings.Contains(output, "?") {
		t.Error("output missing key \"?\"")
	}
}

func TestRenderHelpOverlay_Empty(t *testing.T) {
	// Empty HelpContent should not panic and return minimal output.
	output := RenderHelpOverlay(shared.HelpContent{}, 120, 40)
	if output == "" {
		t.Error("expected non-empty output (at least top padding)")
	}
}

func TestRenderHelpOverlay_SectionsOnly(t *testing.T) {
	// Help overlay no longer renders title/description (sub-header provides those).
	// It renders only key binding sections.
	hc := shared.HelpContent{
		Title:       "Dashboard Help",
		Description: "Lists all pending proposals.",
		Sections: []shared.HelpSection{
			{
				Title: "Navigation",
				Entries: []shared.HelpEntry{
					{Key: "↑", Desc: "Move up"},
				},
			},
		},
	}

	output := RenderHelpOverlay(hc, 120, 40)

	// Title and description should NOT be in the output (they're in the sub-header now).
	if strings.Contains(output, "Dashboard Help") {
		t.Error("output should not contain view title (now in sub-header)")
	}
	if strings.Contains(output, "Lists all pending proposals.") {
		t.Error("output should not contain description (now in sub-header)")
	}

	// Section titles and entries should still be present.
	if !strings.Contains(output, "Navigation") {
		t.Error("output missing section title")
	}
}
