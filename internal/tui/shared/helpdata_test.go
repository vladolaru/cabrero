package shared

import (
	"testing"

	"github.com/vladolaru/cabrero/internal/tui/message"
)

func TestHelpForView_AllViewsCovered(t *testing.T) {
	km := NewKeyMap("arrows")

	views := []message.ViewState{
		message.ViewDashboard,
		message.ViewProposalDetail,
		message.ViewFitnessDetail,
		message.ViewSourceManager,
		message.ViewSourceDetail,
		message.ViewPipelineMonitor,
		message.ViewLogViewer,
	}

	for _, v := range views {
		sections := HelpForView(v, km)
		if len(sections) < 2 {
			t.Errorf("ViewState %d: got %d sections, want at least 2", v, len(sections))
		}
	}
}

func TestHelpForView_DashboardHasAllSections(t *testing.T) {
	km := NewKeyMap("arrows")
	sections := HelpForView(message.ViewDashboard, km)

	want := []string{"Navigation", "Actions", "Views", "Global"}
	if len(sections) != len(want) {
		t.Fatalf("got %d sections, want %d", len(sections), len(want))
	}
	for i, s := range sections {
		if s.Title != want[i] {
			t.Errorf("section[%d].Title = %q, want %q", i, s.Title, want[i])
		}
	}
}

func TestHelpForView_KeysMatchNavMode(t *testing.T) {
	vim := NewKeyMap("vim")
	arrows := NewKeyMap("arrows")

	vimSections := HelpForView(message.ViewDashboard, vim)
	arrowSections := HelpForView(message.ViewDashboard, arrows)

	// Vim nav section should contain "k/↑".
	vimNav := vimSections[0]
	found := false
	for _, e := range vimNav.Entries {
		if e.Key == "k/↑" {
			found = true
			break
		}
	}
	if !found {
		t.Error("vim mode: expected navigation entry with key \"k/↑\"")
	}

	// Arrows nav section should contain "↑".
	arrowNav := arrowSections[0]
	found = false
	for _, e := range arrowNav.Entries {
		if e.Key == "↑" {
			found = true
			break
		}
	}
	if !found {
		t.Error("arrows mode: expected navigation entry with key \"↑\"")
	}
}

func TestHelpForView_NoDuplicateKeys(t *testing.T) {
	km := NewKeyMap("arrows")

	views := []message.ViewState{
		message.ViewDashboard,
		message.ViewProposalDetail,
		message.ViewFitnessDetail,
		message.ViewSourceManager,
		message.ViewSourceDetail,
		message.ViewPipelineMonitor,
		message.ViewLogViewer,
	}

	for _, v := range views {
		sections := HelpForView(v, km)
		seen := make(map[string]bool)
		for _, s := range sections {
			for _, e := range s.Entries {
				if seen[e.Key] {
					t.Errorf("ViewState %d: duplicate key %q", v, e.Key)
				}
				seen[e.Key] = true
			}
		}
	}
}
