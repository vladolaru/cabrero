package shared

import (
	"strings"
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
		hc := HelpForView(v, km)
		if len(hc.Sections) < 2 {
			t.Errorf("ViewState %d: got %d sections, want at least 2", v, len(hc.Sections))
		}
	}
}

func TestHelpForView_DashboardHasAllSections(t *testing.T) {
	km := NewKeyMap("arrows")
	hc := HelpForView(message.ViewDashboard, km)

	want := []string{"Navigation", "Actions", "Views", "Global"}
	if len(hc.Sections) != len(want) {
		t.Fatalf("got %d sections, want %d", len(hc.Sections), len(want))
	}
	for i, s := range hc.Sections {
		if s.Title != want[i] {
			t.Errorf("section[%d].Title = %q, want %q", i, s.Title, want[i])
		}
	}
}

func TestHelpForView_KeysMatchNavMode(t *testing.T) {
	vim := NewKeyMap("vim")
	arrows := NewKeyMap("arrows")

	vimHC := HelpForView(message.ViewDashboard, vim)
	arrowHC := HelpForView(message.ViewDashboard, arrows)

	// Vim nav section should contain "k/↑".
	vimNav := vimHC.Sections[0]
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
	arrowNav := arrowHC.Sections[0]
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

func TestLogViewerHelpHasExpandCollapse(t *testing.T) {
	km := NewKeyMap("vim")
	hc := HelpForView(message.ViewLogViewer, km)
	found := false
	for _, sec := range hc.Sections {
		for _, entry := range sec.Entries {
			if strings.Contains(entry.Desc, "expand") || strings.Contains(entry.Desc, "Expand") {
				found = true
			}
		}
	}
	if !found {
		t.Error("log viewer help should include expand/collapse entries")
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
		hc := HelpForView(v, km)
		seen := make(map[string]bool)
		for _, s := range hc.Sections {
			for _, e := range s.Entries {
				if seen[e.Key] {
					t.Errorf("ViewState %d: duplicate key %q", v, e.Key)
				}
				seen[e.Key] = true
			}
		}
	}
}

func TestHelpForView_AllViewsHaveTitleAndDescription(t *testing.T) {
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
		hc := HelpForView(v, km)
		if hc.Title == "" {
			t.Errorf("ViewState %d: Title is empty", v)
		}
		if hc.Description == "" {
			t.Errorf("ViewState %d: Description is empty", v)
		}
		if !strings.HasSuffix(hc.Title, "Help") {
			t.Errorf("ViewState %d: Title %q should end with \"Help\"", v, hc.Title)
		}
	}
}
