package fitness

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/fitness"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// testReport returns a fitness report with sensible defaults for testing.
func testReport() *fitness.Report {
	now := time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)
	return &fitness.Report{
		ID:            "report-001",
		SourceName:    "docx-helper",
		SourceOrigin:  "user",
		Ownership:     "mine",
		ObservedCount: 12,
		WindowDays:    30,
		Assessment: fitness.Assessment{
			Followed:     fitness.BucketStat{Count: 8, Percent: 66.7},
			WorkedAround: fitness.BucketStat{Count: 3, Percent: 25.0},
			Confused:     fitness.BucketStat{Count: 1, Percent: 8.3},
		},
		Verdict: "This skill is mostly followed but has some workaround patterns.",
		Evidence: []fitness.EvidenceGroup{
			{
				Category: "followed",
				Entries: []fitness.EvidenceEntry{
					{
						SessionID: "sess-1",
						Timestamp: now.Add(-24 * time.Hour),
						Summary:   "Used SKILL.md workflow correctly",
						Detail:    "Read template before writing output.",
					},
					{
						SessionID: "sess-2",
						Timestamp: now.Add(-48 * time.Hour),
						Summary:   "Followed all steps in order",
					},
				},
			},
			{
				Category: "worked_around",
				Entries: []fitness.EvidenceEntry{
					{
						SessionID: "sess-3",
						Timestamp: now.Add(-72 * time.Hour),
						Summary:   "Skipped template read step",
						Detail:    "Wrote output without reading template first.",
					},
				},
			},
			{
				Category: "confused",
				Entries: []fitness.EvidenceEntry{
					{
						SessionID: "sess-4",
						Timestamp: now.Add(-96 * time.Hour),
						Summary:   "Multiple retries on output format",
					},
				},
			},
		},
		GeneratedAt: now,
	}
}

func newTestFitness() Model {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	report := testReport()
	m := New(report, &keys, cfg)
	m.SetSize(120, 40)
	return m
}

func TestFitness_EvidenceExpand(t *testing.T) {
	m := newTestFitness()

	// Initially no evidence group is expanded.
	if m.evidence[0].Expanded {
		t.Fatal("first evidence group should start collapsed")
	}

	// selectedEvidence starts at 0 (first group).
	if m.selectedEvidence != 0 {
		t.Fatalf("selectedEvidence = %d, want 0", m.selectedEvidence)
	}

	// Press Enter to expand the first evidence group.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.evidence[0].Expanded {
		t.Error("first evidence group should be expanded after Enter")
	}

	// Verify that the viewport content includes the evidence entries.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Used SKILL.md workflow correctly") {
		t.Error("expanded evidence should show entry summaries in viewport")
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.evidence[0].Expanded {
		t.Error("first evidence group should be collapsed after second Enter")
	}

	// Move cursor down to second group and expand it.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedEvidence != 1 {
		t.Fatalf("selectedEvidence after Down = %d, want 1", m.selectedEvidence)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.evidence[1].Expanded {
		t.Error("second evidence group should be expanded")
	}
	if m.evidence[0].Expanded {
		t.Error("first evidence group should still be collapsed")
	}

	// Verify original report is NOT mutated.
	report := testReport()
	for i, eg := range report.Evidence {
		if eg.Expanded {
			t.Errorf("original report evidence[%d].Expanded should be false", i)
		}
	}
}

func TestFitness_DismissEmitsMessage(t *testing.T) {
	m := newTestFitness()

	// Press 'x' to dismiss.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})

	if cmd == nil {
		t.Fatal("dismiss should produce a cmd")
	}

	msg := cmd()
	dismiss, ok := msg.(message.DismissFinished)
	if !ok {
		t.Fatalf("expected DismissFinished, got %T", msg)
	}
	if dismiss.ReportID != "report-001" {
		t.Errorf("ReportID = %q, want %q", dismiss.ReportID, "report-001")
	}
	if dismiss.Err != nil {
		t.Errorf("Err = %v, want nil", dismiss.Err)
	}

	_ = m // suppress unused
}

func TestFitness_JumpToSources(t *testing.T) {
	m := newTestFitness()

	// Press 's' to jump to sources.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	if cmd == nil {
		t.Fatal("jump to sources should produce a cmd")
	}

	msg := cmd()
	jump, ok := msg.(message.JumpToSources)
	if !ok {
		t.Fatalf("expected JumpToSources, got %T", msg)
	}
	if jump.SourceName != "docx-helper" {
		t.Errorf("SourceName = %q, want %q", jump.SourceName, "docx-helper")
	}

	_ = m // suppress unused
}

func TestFitness_FocusToggle(t *testing.T) {
	m := newTestFitness()

	if m.focus != FocusReport {
		t.Fatalf("initial focus = %d, want FocusReport", m.focus)
	}

	// Tab switches to chat.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusChat {
		t.Errorf("focus after Tab = %d, want FocusChat", m.focus)
	}

	// Tab switches back.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusReport {
		t.Errorf("focus after second Tab = %d, want FocusReport", m.focus)
	}
}

func TestFitness_ChatKey(t *testing.T) {
	m := newTestFitness()

	// Press 'c' to focus chat.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.focus != FocusChat {
		t.Errorf("focus after 'c' = %d, want FocusChat", m.focus)
	}
}

func TestFitness_BackEmitsPopView(t *testing.T) {
	m := newTestFitness()

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("Esc should emit a cmd")
	}
	msg := cmd()
	if _, ok := msg.(message.PopView); !ok {
		t.Fatalf("expected PopView, got %T", msg)
	}
}

func TestFitness_View(t *testing.T) {
	m := newTestFitness()
	view := ansi.Strip(m.View())

	// Report header is now in SubHeader(), rendered by root model.
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Fitness Report") {
		t.Error("missing report title in sub-header")
	}
	if !strings.Contains(subHeader, "docx-helper") {
		t.Error("missing source name in sub-header")
	}
	if !strings.Contains(view, "ASSESSMENT") {
		t.Error("missing assessment section")
	}
	if !strings.Contains(view, "VERDICT") {
		t.Error("missing verdict section")
	}
	if !strings.Contains(view, "SESSION EVIDENCE") {
		t.Error("missing session evidence section")
	}
	if !strings.Contains(view, "Followed Correctly") {
		t.Error("missing evidence category label")
	}
}

func TestFitness_NilReport(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := shared.DefaultConfig()
	m := New(nil, &keys, cfg)
	m.SetSize(80, 24)

	view := m.View()
	if !strings.Contains(view, "No report selected") {
		t.Error("nil report should show 'No report selected'")
	}

	// Dismiss should be a no-op.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd != nil {
		t.Error("dismiss on nil report should produce no cmd")
	}

	// Jump to sources should be a no-op.
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd != nil {
		t.Error("jump to sources on nil report should produce no cmd")
	}
}

func TestFitness_EvidenceNavigation(t *testing.T) {
	m := newTestFitness()

	// Start at 0.
	if m.selectedEvidence != 0 {
		t.Fatalf("initial selectedEvidence = %d, want 0", m.selectedEvidence)
	}

	// Move down to second group.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedEvidence != 1 {
		t.Errorf("selectedEvidence after Down = %d, want 1", m.selectedEvidence)
	}

	// Move down to third group.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedEvidence != 2 {
		t.Errorf("selectedEvidence after second Down = %d, want 2", m.selectedEvidence)
	}

	// At bottom, should not go further.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.selectedEvidence != 2 {
		t.Errorf("selectedEvidence should stay at 2 (bottom), got %d", m.selectedEvidence)
	}

	// Move up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedEvidence != 1 {
		t.Errorf("selectedEvidence after Up = %d, want 1", m.selectedEvidence)
	}

	// At top, should not go further.
	m.selectedEvidence = 0
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.selectedEvidence != 0 {
		t.Errorf("selectedEvidence should stay at 0 (top), got %d", m.selectedEvidence)
	}
}
