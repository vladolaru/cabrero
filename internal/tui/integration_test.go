package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func newTestRoot() reviewModel {
	proposals := testdata.TestProposals()
	reports := testdata.TestFitnessReports()
	stats := testdata.TestDashboardStats()
	sourceGroups := testdata.TestSourceGroups()
	runs := testdata.TestPipelineRuns()
	pipelineStats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	cfg := testdata.TestConfig()
	return newReviewModel(proposals, reports, stats, sourceGroups, runs, pipelineStats, prompts, cfg)
}

// update is a helper that calls Update and returns the concrete reviewModel.
func update(m reviewModel, msg tea.Msg) (reviewModel, tea.Cmd) {
	model, cmd := m.Update(msg)
	return model.(reviewModel), cmd
}

func TestFullNavigationFlow(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Start at dashboard.
	if m.state != message.ViewDashboard {
		t.Fatalf("initial state = %d, want ViewDashboard", m.state)
	}

	// Dashboard should render.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Cabrero Review") {
		t.Error("dashboard missing title")
	}

	// Navigate down, then press Enter to open detail.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyEnter})

	// Enter should produce PushView cmd.
	if cmd == nil {
		t.Fatal("Enter should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewProposalDetail {
		t.Errorf("PushView = %d, want ViewProposalDetail", push.View)
	}

	// Process the PushView message.
	m, _ = update(m, push)
	if m.state != message.ViewProposalDetail {
		t.Errorf("state after push = %d, want ViewProposalDetail", m.state)
	}
	if len(m.viewStack) != 1 {
		t.Errorf("viewStack len = %d, want 1", len(m.viewStack))
	}

	// Detail should render proposal content.
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Proposal:") {
		t.Error("detail missing proposal header")
	}

	// Press Esc to go back.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		popMsg := cmd()
		m, _ = update(m, popMsg)
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}
	if len(m.viewStack) != 0 {
		t.Errorf("viewStack len = %d, want 0", len(m.viewStack))
	}
}

func TestQuitFromDashboard(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Press 'q' should quit from dashboard.
	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q should produce quit cmd")
	}

	msg := cmd()
	if msg != tea.Quit() {
		t.Errorf("expected Quit message, got %T", msg)
	}
}

func TestQuitBlockedFromDetail(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to detail view.
	m, _ = update(m, message.PushView{View: message.ViewProposalDetail})
	if m.state != message.ViewProposalDetail {
		t.Fatal("should be in detail view")
	}

	// Press 'q' should NOT quit from detail.
	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd != nil {
		msg := cmd()
		if msg == tea.Quit() {
			t.Error("q should not quit from detail view")
		}
	}
}

func TestForceQuitFromAnywhere(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to detail.
	m, _ = update(m, message.PushView{View: message.ViewProposalDetail})

	// Ctrl+C should force quit.
	_, cmd := update(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should produce quit cmd")
	}

	msg := cmd()
	if msg != tea.Quit() {
		t.Errorf("expected Quit message, got %T", msg)
	}
}

func TestHelpOverlayToggle(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	if m.helpOpen {
		t.Fatal("help should start closed")
	}

	// Press '?' to open help.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.helpOpen {
		t.Error("help should be open after ?")
	}

	// Press '?' again to close.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.helpOpen {
		t.Error("help should be closed after second ?")
	}
}

func TestStatusMessageExpiry(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	// Send a reject finished message.
	m, _ = update(m, message.RejectFinished{ProposalID: "test"})
	if m.statusMsg == "" {
		t.Error("status message should be set after reject")
	}

	// Force expiry by setting time in the past.
	m.statusExpiry = time.Now().Add(-1 * time.Second)
	m, _ = update(m, message.StatusMessageExpired{})
	if m.statusMsg != "" {
		t.Error("status message should be cleared after expiry")
	}
}

func TestViewStackPreservation(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to detail.
	m, _ = update(m, message.PushView{View: message.ViewProposalDetail})
	if m.state != message.ViewProposalDetail {
		t.Fatal("should be in detail")
	}

	// Pop back.
	m, _ = update(m, message.PopView{})
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}

	// Pop again should be no-op (already at root).
	m, _ = update(m, message.PopView{})
	if m.state != message.ViewDashboard {
		t.Errorf("state after extra pop = %d, want ViewDashboard", m.state)
	}
}

// Phase 4b integration tests.

func TestDashboardToFitnessAndBack(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Navigate to a fitness report item (proposals are first, reports after).
	// Our test data has 3 proposals then 2 fitness reports, so cursor 3 = first report.
	for i := 0; i < 3; i++ {
		m, _ = update(m, tea.KeyMsg{Type: tea.KeyDown})
	}

	// Press Enter to open fitness detail.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on fitness report should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewFitnessDetail {
		t.Errorf("PushView = %d, want ViewFitnessDetail", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewFitnessDetail {
		t.Errorf("state = %d, want ViewFitnessDetail", m.state)
	}

	// Fitness view should render.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "docx-helper") {
		t.Error("fitness view should contain source name")
	}

	// Press Esc to go back.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		popMsg := cmd()
		m, _ = update(m, popMsg)
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}
}

func TestDashboardToSourceManager(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press 's' to open source manager.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("s should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewSourceManager {
		t.Errorf("PushView = %d, want ViewSourceManager", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewSourceManager {
		t.Errorf("state = %d, want ViewSourceManager", m.state)
	}

	// Source manager should render.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "SOURCE") {
		t.Error("source manager view should contain column header")
	}

	// Press Esc to go back.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		msg := cmd()
		m, _ = update(m, msg)
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}
}

func TestFitnessJumpToSources(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Navigate to a fitness report (3 proposals, then fitness reports).
	for i := 0; i < 3; i++ {
		m, _ = update(m, tea.KeyMsg{Type: tea.KeyDown})
	}

	// Push to fitness detail via Enter.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should produce cmd")
	}
	m, _ = update(m, cmd())
	if m.state != message.ViewFitnessDetail {
		t.Fatal("should be in fitness detail")
	}

	// Press 's' to jump to sources.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if cmd == nil {
		t.Fatal("s in fitness view should produce cmd")
	}

	// The fitness model emits JumpToSources which the root handles.
	jumpMsg := cmd()
	jump, ok := jumpMsg.(message.JumpToSources)
	if !ok {
		t.Fatalf("expected JumpToSources, got %T", jumpMsg)
	}

	// Process the jump.
	m, _ = update(m, jump)
	if m.state != message.ViewSourceManager {
		t.Errorf("state = %d, want ViewSourceManager", m.state)
	}
	if len(m.viewStack) != 2 {
		t.Errorf("viewStack len = %d, want 2 (dashboard + fitness)", len(m.viewStack))
	}
}

func TestSourceManagerGroupCollapse(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to source manager.
	m, _ = update(m, message.PushView{View: message.ViewSourceManager})
	if m.state != message.ViewSourceManager {
		t.Fatal("should be in source manager")
	}

	// Get initial view.
	viewBefore := ansi.Strip(m.View())

	// Press Left to collapse current group.
	m, _ = update(m, tea.KeyMsg{Type: tea.KeyLeft})

	// View should change (group collapsed, fewer lines).
	viewAfter := ansi.Strip(m.View())
	if viewBefore == viewAfter {
		t.Error("collapsing group should change the view")
	}
}

// Phase 4c integration tests.

func TestDashboardToPipelineAndBack(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Press 'p' to open pipeline monitor.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if cmd == nil {
		t.Fatal("p should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewPipelineMonitor {
		t.Errorf("PushView = %d, want ViewPipelineMonitor", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("state = %d, want ViewPipelineMonitor", m.state)
	}

	// Pipeline monitor should render.
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "DAEMON") || !strings.Contains(view, "RECENT RUNS") {
		t.Error("pipeline monitor view missing expected sections")
	}

	// Press Esc to go back.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after pop = %d, want ViewDashboard", m.state)
	}
}

func TestPipelineToLogViewerAndBack(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Push to pipeline monitor.
	m, _ = update(m, message.PushView{View: message.ViewPipelineMonitor})
	if m.state != message.ViewPipelineMonitor {
		t.Fatal("should be in pipeline monitor")
	}

	// Press 'L' to open log viewer.
	m, cmd := update(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd == nil {
		t.Fatal("L should produce cmd")
	}
	pushMsg := cmd()
	push, ok := pushMsg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", pushMsg)
	}
	if push.View != message.ViewLogViewer {
		t.Errorf("PushView = %d, want ViewLogViewer", push.View)
	}

	// Process the push.
	m, _ = update(m, push)
	if m.state != message.ViewLogViewer {
		t.Errorf("state = %d, want ViewLogViewer", m.state)
	}
	if len(m.viewStack) != 2 {
		t.Errorf("viewStack len = %d, want 2", len(m.viewStack))
	}

	// Press Esc to go back to pipeline.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("state after pop = %d, want ViewPipelineMonitor", m.state)
	}

	// Press Esc again to go back to dashboard.
	m, cmd = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if cmd != nil {
		m, _ = update(m, cmd())
	}
	if m.state != message.ViewDashboard {
		t.Errorf("state after second pop = %d, want ViewDashboard", m.state)
	}
}

func TestFullStackNavigation(t *testing.T) {
	m := newTestRoot()
	m, _ = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Dashboard -> Pipeline -> Log -> back -> back -> Dashboard.
	m, _ = update(m, message.PushView{View: message.ViewPipelineMonitor})
	m, _ = update(m, message.PushView{View: message.ViewLogViewer})

	if m.state != message.ViewLogViewer {
		t.Fatal("should be in log viewer")
	}
	if len(m.viewStack) != 2 {
		t.Fatalf("viewStack = %d, want 2", len(m.viewStack))
	}

	m, _ = update(m, message.PopView{})
	if m.state != message.ViewPipelineMonitor {
		t.Errorf("after first pop: state = %d, want ViewPipelineMonitor", m.state)
	}

	m, _ = update(m, message.PopView{})
	if m.state != message.ViewDashboard {
		t.Errorf("after second pop: state = %d, want ViewDashboard", m.state)
	}
}
