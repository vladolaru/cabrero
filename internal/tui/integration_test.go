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
	stats := testdata.TestDashboardStats()
	sourceGroups := testdata.TestSourceGroups()
	cfg := testdata.TestConfig()
	return newReviewModel(proposals, stats, sourceGroups, cfg)
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
