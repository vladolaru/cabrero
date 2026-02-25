package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/pipeline"
	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func TestDashboardItem_FilterValue(t *testing.T) {
	p := testdata.TestProposal()
	item := DashboardItem{Proposal: &p}

	fv := item.FilterValue()
	if !strings.Contains(fv, item.TypeName()) {
		t.Errorf("FilterValue %q should contain TypeName %q", fv, item.TypeName())
	}
	if !strings.Contains(fv, "target:") {
		t.Error("FilterValue should contain 'target:' tag")
	}
	if !strings.Contains(fv, "type:") {
		t.Error("FilterValue should contain 'type:' tag")
	}
}

func TestDashboard_ListIndex_Navigation(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.list.Index() != 0 {
		t.Fatalf("initial index = %d, want 0", m.list.Index())
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.list.Index() != 1 {
		t.Errorf("index after down = %d, want 1", m.list.Index())
	}
}

func TestDashboard_HasActiveInput_WhenFiltering(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.HasActiveInput() {
		t.Error("HasActiveInput should be false initially")
	}

	// Open filter.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.HasActiveInput() {
		t.Error("HasActiveInput should be true while filtering")
	}
}

func newTestModel() Model {
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	return New(testdata.TestProposals(), testdata.TestFitnessReports(), testdata.TestDashboardStats(), &keys, cfg)
}

func TestDashboard_Navigation(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Start at top.
	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.cursor)
	}

	// Total items: 3 proposals + 2 fitness reports = 5.
	totalItems := len(m.filtered)
	if totalItems != 5 {
		t.Fatalf("total items = %d, want 5", totalItems)
	}

	// Move down through all items.
	for i := 1; i < totalItems; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		if m.cursor != i {
			t.Errorf("cursor after down %d = %d, want %d", i, m.cursor, i)
		}
	}

	// At bottom — should not go further.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != totalItems-1 {
		t.Errorf("cursor should stay at %d (bottom), got %d", totalItems-1, m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != totalItems-2 {
		t.Errorf("cursor after up = %d, want %d", m.cursor, totalItems-2)
	}

	// Go to top.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m.cursor != 0 {
		t.Errorf("cursor after home = %d, want 0", m.cursor)
	}

	// Go to bottom.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.cursor != totalItems-1 {
		t.Errorf("cursor after end = %d, want %d", m.cursor, totalItems-1)
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
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p = m.SelectedProposal()
	if p == nil {
		t.Fatal("SelectedProposal returned nil at index 1")
	}
	if p.Proposal.ID != "prop-scaffold-001" {
		t.Errorf("second proposal ID = %q, want %q", p.Proposal.ID, "prop-scaffold-001")
	}
}

func TestDashboard_SelectedItem_FitnessReport(t *testing.T) {
	m := newTestModel()

	// Move past the 3 proposals to the first fitness report (index 3).
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	item := m.SelectedItem()
	if item == nil {
		t.Fatal("SelectedItem returned nil at fitness report index")
	}
	if !item.IsFitnessReport() {
		t.Error("expected fitness report at index 3")
	}
	if item.FitnessReport.ID != "fit-001" {
		t.Errorf("fitness report ID = %q, want %q", item.FitnessReport.ID, "fit-001")
	}

	// SelectedProposal should return nil for fitness reports.
	if m.SelectedProposal() != nil {
		t.Error("SelectedProposal should return nil when cursor is on fitness report")
	}

	// SelectedFitnessReport should return the report.
	fr := m.SelectedFitnessReport()
	if fr == nil {
		t.Fatal("SelectedFitnessReport returned nil")
	}
	if fr.ID != "fit-001" {
		t.Errorf("SelectedFitnessReport ID = %q, want %q", fr.ID, "fit-001")
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
		m, _ = m.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
		if m.sortOrder != want {
			t.Errorf("sort = %q, want %q", m.sortOrder, want)
		}
	}
}

func TestDashboard_EmptyState(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	m := New(nil, nil, testdata.TestDashboardStatsEmpty(), &keys, cfg)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "The flock is calm") {
		t.Error("empty state should show flavor text")
	}
}

func TestDashboard_EmptySelectedProposal(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()
	m := New(nil, nil, testdata.TestDashboardStatsEmpty(), &keys, cfg)

	p := m.SelectedProposal()
	if p != nil {
		t.Error("SelectedProposal should be nil when empty")
	}

	item := m.SelectedItem()
	if item != nil {
		t.Error("SelectedItem should be nil when empty")
	}
}

func TestDashboard_View80x24(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())

	// Should contain key elements (header and sub-header are rendered by root model).
	if !strings.Contains(view, "skill_improvement") {
		t.Error("missing proposal type")
	}
	if !strings.Contains(view, "skill_scaffold") {
		t.Error("missing scaffold proposal")
	}
	if !strings.Contains(view, "fitness_report") {
		t.Error("missing fitness report type")
	}
	if !strings.Contains(view, "Sort:") {
		t.Error("missing sort indicator")
	}
	if !strings.Contains(view, "TYPE") {
		t.Error("missing column headers")
	}
}

func TestDashboard_OpenSendsMessage(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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

func TestDashboard_OpenFitnessReport(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Move to first fitness report (index 3).
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on fitness report should produce a cmd")
	}

	msg := cmd()
	push, ok := msg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", msg)
	}
	if push.View != message.ViewFitnessDetail {
		t.Errorf("PushView.View = %d, want ViewFitnessDetail", push.View)
	}
}

func TestDashboard_SourcesKeySendsMessage(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("'s' should produce a cmd")
	}

	msg := cmd()
	push, ok := msg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", msg)
	}
	if push.View != message.ViewSourceManager {
		t.Errorf("PushView.View = %d, want ViewSourceManager", push.View)
	}
}

func TestDashboard_ActionKeysGatedOnProposal(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Move to first fitness report (index 3).
	for i := 0; i < 3; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// Approve, Reject, Defer should produce nil cmd on a fitness report.
	for _, r := range []rune{'a', 'r', 'd'} {
		_, cmd := m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		if cmd != nil {
			t.Errorf("key '%c' should not produce cmd on fitness report", r)
		}
	}
}

func TestDashboard_ViewportScrolls(t *testing.T) {
	// Create a model with many proposals so items exceed viewport height.
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()

	// Generate 20 proposals.
	proposals := make([]pipeline.ProposalWithSession, 20)
	for i := range proposals {
		proposals[i] = testdata.TestProposal(func(p *pipeline.Proposal) {
			p.ID = fmt.Sprintf("prop-%03d", i)
		})
	}

	m := New(proposals, nil, testdata.TestDashboardStats(), &keys, cfg)
	// Height 8, chrome = 3 (column header + sort indicator + status bar) → viewport = 5.
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})

	if m.viewport.Height() != 5 {
		t.Fatalf("viewport height = %d, want 5", m.viewport.Height())
	}

	// Cursor starts at 0, viewport at top.
	if m.viewport.YOffset() != 0 {
		t.Errorf("initial YOffset = %d, want 0", m.viewport.YOffset())
	}

	// Move cursor down past the viewport.
	for i := 0; i < 10; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != 10 {
		t.Fatalf("cursor = %d, want 10", m.cursor)
	}

	// Cursor line maps directly to viewport row (no header offset).
	cursorLine := m.cursor
	if cursorLine < m.viewport.YOffset() || cursorLine >= m.viewport.YOffset()+m.viewport.Height() {
		t.Errorf("cursor line %d not visible: YOffset=%d Height=%d", cursorLine, m.viewport.YOffset(), m.viewport.Height())
	}

	// GotoBottom should show last item.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.cursor != 19 {
		t.Fatalf("cursor after End = %d, want 19", m.cursor)
	}
	cursorLine = m.cursor
	if cursorLine < m.viewport.YOffset() || cursorLine >= m.viewport.YOffset()+m.viewport.Height() {
		t.Errorf("cursor line %d not visible after End: YOffset=%d", cursorLine, m.viewport.YOffset())
	}

	// GotoTop should scroll back to top.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	if m.cursor != 0 {
		t.Fatalf("cursor after Home = %d, want 0", m.cursor)
	}
	if m.viewport.YOffset() != 0 {
		t.Errorf("YOffset after Home = %d, want 0", m.viewport.YOffset())
	}
}

func TestDashboard_HalfPageScroll(t *testing.T) {
	keys := shared.NewKeyMap("arrows")
	cfg := testdata.TestConfig()

	proposals := make([]pipeline.ProposalWithSession, 30)
	for i := range proposals {
		proposals[i] = testdata.TestProposal(func(p *pipeline.Proposal) {
			p.ID = fmt.Sprintf("prop-%03d", i)
		})
	}

	m := New(proposals, nil, testdata.TestDashboardStats(), &keys, cfg)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 21}) // viewport height = 18 (21 - 3 chrome)
	halfPage := m.viewport.Height() / 2                          // 9

	// PgDn moves cursor by half a page.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.cursor != halfPage {
		t.Errorf("cursor after PgDn = %d, want %d", m.cursor, halfPage)
	}

	// PgUp moves back.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.cursor != 0 {
		t.Errorf("cursor after PgUp = %d, want 0", m.cursor)
	}

	// PgUp at top stays at 0.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if m.cursor != 0 {
		t.Errorf("cursor after PgUp at top = %d, want 0", m.cursor)
	}
}

func TestDashboardItem_Methods(t *testing.T) {
	m := newTestModel()

	// First item is a proposal.
	item := m.filtered[0]
	if !item.IsProposal() {
		t.Error("first item should be a proposal")
	}
	if item.IsFitnessReport() {
		t.Error("first item should not be a fitness report")
	}
	if item.TypeIndicator() != indicatorProposal {
		t.Errorf("proposal indicator = %q, want %q", item.TypeIndicator(), indicatorProposal)
	}
	if item.TypeName() != "skill_improvement" {
		t.Errorf("proposal TypeName = %q, want %q", item.TypeName(), "skill_improvement")
	}

	// Fourth item (index 3) is a fitness report.
	fitnessItem := m.filtered[3]
	if !fitnessItem.IsFitnessReport() {
		t.Error("item at index 3 should be a fitness report")
	}
	if fitnessItem.IsProposal() {
		t.Error("item at index 3 should not be a proposal")
	}
	if fitnessItem.TypeIndicator() != indicatorFitness {
		t.Errorf("fitness indicator = %q, want %q", fitnessItem.TypeIndicator(), indicatorFitness)
	}
	if fitnessItem.TypeName() != "fitness_report" {
		t.Errorf("fitness TypeName = %q, want %q", fitnessItem.TypeName(), "fitness_report")
	}
	if fitnessItem.Target() != "docx-helper" {
		t.Errorf("fitness Target = %q, want %q", fitnessItem.Target(), "docx-helper")
	}
}

func TestDashboard_StatusMessage_Shown(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, cmd := m.Update(message.StatusMessage{Text: "Change applied.", Duration: 3 * time.Second})
	if m.statusMsg != "Change applied." {
		t.Errorf("statusMsg = %q, want %q", m.statusMsg, "Change applied.")
	}
	if cmd == nil {
		t.Fatal("expected expiry tick cmd")
	}

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Change applied.") {
		t.Error("View() should contain the status message")
	}
}

func TestDashboard_StatusMessage_Expiry(t *testing.T) {
	m := newTestModel()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m, _ = m.Update(message.StatusMessage{Text: "hello", Duration: 3 * time.Second})
	if m.statusMsg != "hello" {
		t.Fatalf("statusMsg = %q", m.statusMsg)
	}

	// Simulate expiry: travel past the deadline.
	m.statusExpiry = time.Now().Add(-1 * time.Second)
	m, _ = m.Update(message.StatusMessageExpired{})
	if m.statusMsg != "" {
		t.Errorf("statusMsg should be cleared, got %q", m.statusMsg)
	}
}
