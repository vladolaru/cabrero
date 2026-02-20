package pipeline

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/message"
	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

func newTestModel() Model {
	runs := testdata.TestPipelineRuns()
	stats := testdata.TestPipelineStats()
	prompts := testdata.TestPromptVersions()
	dashStats := testdata.TestDashboardStats()
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	return New(runs, stats, prompts, dashStats, &keys, cfg)
}

func TestNewModel(t *testing.T) {
	m := newTestModel()
	if len(m.runs) != 4 {
		t.Errorf("runs = %d, want 4", len(m.runs))
	}
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
}

func TestModelView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "DAEMON") {
		t.Error("view missing DAEMON section")
	}
	if !strings.Contains(view, "RECENT RUNS") {
		t.Error("view missing RECENT RUNS section")
	}
	if !strings.Contains(view, "e7f2a103") {
		t.Error("view missing first run session ID")
	}
}

func TestModelNavigation(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Move down.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0", m.cursor)
	}
}

func TestModelExpandRun(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Press Enter to expand.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.expandedIdx != 0 {
		t.Errorf("expandedIdx = %d, want 0", m.expandedIdx)
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.expandedIdx != -1 {
		t.Errorf("expandedIdx = %d, want -1", m.expandedIdx)
	}
}

func TestModelRetryKey(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Navigate to errored run (index 2).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Press R.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})

	// Should activate confirm — note: .Active is a FIELD, not a method.
	if !m.confirm.Active {
		t.Error("R on errored run should activate confirm")
	}
}

func TestModelPipelineKeyEmitsPushLogView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Press L.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'L'}})
	if cmd == nil {
		t.Fatal("L should produce cmd")
	}
	msg := cmd()
	push, ok := msg.(message.PushView)
	if !ok {
		t.Fatalf("expected PushView, got %T", msg)
	}
	if push.View != message.ViewLogViewer {
		t.Errorf("push view = %d, want ViewLogViewer", push.View)
	}
}
