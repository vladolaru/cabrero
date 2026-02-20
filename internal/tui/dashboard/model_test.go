package dashboard

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func newTestModel() Model {
	keys := tui.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	return New(testdata.TestProposals(), testdata.TestDashboardStats(), &keys, cfg)
}

func TestDashboard_Navigation(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Start at top.
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	// Move down.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}

	// Move down again.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor after second down = %d, want 2", m.cursor)
	}

	// At bottom — should not go further.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 2 {
		t.Errorf("cursor should stay at 2 (bottom), got %d", m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 1 {
		t.Errorf("cursor after up = %d, want 1", m.cursor)
	}

	// Go to top.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if m.cursor != 0 {
		t.Errorf("cursor after home = %d, want 0", m.cursor)
	}

	// Go to bottom.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.cursor != 2 {
		t.Errorf("cursor after end = %d, want 2", m.cursor)
	}
}

func TestDashboard_SelectedProposal(t *testing.T) {
	m := newTestModel()

	p := m.SelectedProposal()
	if p == nil {
		t.Fatal("SelectedProposal returned nil")
	}
	if p.Proposal.ID != "prop-abc123" {
		t.Errorf("first proposal ID = %q, want %q", p.Proposal.ID, "prop-abc123")
	}

	// Move to second.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	p = m.SelectedProposal()
	if p == nil {
		t.Fatal("SelectedProposal returned nil at index 1")
	}
	if p.Proposal.ID != "prop-scaffold-001" {
		t.Errorf("second proposal ID = %q, want %q", p.Proposal.ID, "prop-scaffold-001")
	}
}

func TestDashboard_SortCycle(t *testing.T) {
	m := newTestModel()

	// Default is "newest".
	if m.sortOrder != SortNewest {
		t.Fatalf("initial sort = %q, want %q", m.sortOrder, SortNewest)
	}

	// Cycle: newest -> oldest -> confidence -> type -> newest.
	expected := []string{SortOldest, SortConfidence, SortType, SortNewest}
	for _, want := range expected {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
		if m.sortOrder != want {
			t.Errorf("sort = %q, want %q", m.sortOrder, want)
		}
	}
}

func TestDashboard_EmptyState(t *testing.T) {
	keys := tui.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	m := New(nil, testdata.TestDashboardStatsEmpty(), &keys, cfg)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "The flock is calm") {
		t.Error("empty state should show flavor text")
	}
}

func TestDashboard_EmptySelectedProposal(t *testing.T) {
	keys := tui.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	m := New(nil, testdata.TestDashboardStatsEmpty(), &keys, cfg)

	p := m.SelectedProposal()
	if p != nil {
		t.Error("SelectedProposal should be nil when empty")
	}
}

func TestDashboard_View80x24(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())

	// Should contain key elements.
	if !strings.Contains(view, "Cabrero Review") {
		t.Error("missing title")
	}
	if !strings.Contains(view, "PENDING REVIEW") {
		t.Error("missing section header")
	}
	if !strings.Contains(view, "skill_improvement") {
		t.Error("missing proposal type")
	}
	if !strings.Contains(view, "skill_scaffold") {
		t.Error("missing scaffold proposal")
	}
}

func TestDashboard_OpenSendsMessage(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should produce a cmd")
	}

	msg := cmd()
	push, ok := msg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", msg)
	}
	if push.View != message.ViewProposalDetail {
		t.Errorf("PushView.View = %d, want ViewProposalDetail", push.View)
	}
}
