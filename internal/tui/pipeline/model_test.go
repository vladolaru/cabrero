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

func TestModelViewDaemonHeader(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)
	view := ansi.Strip(m.View())

	// Enriched daemon header fields.
	if !strings.Contains(view, "Uptime:") {
		t.Error("view missing Uptime field")
	}
	if !strings.Contains(view, "Poll:") {
		t.Error("view missing Poll interval field")
	}
	if !strings.Contains(view, "Stale:") {
		t.Error("view missing Stale interval field")
	}

	// Store section.
	if !strings.Contains(view, "STORE") {
		t.Error("view missing STORE section")
	}
	if !strings.Contains(view, "sessions") {
		t.Error("view missing session count")
	}

	// Two-column layout at width >= 120: HOOKS should appear on same visual row as DAEMON.
	lines := strings.Split(view, "\n")
	hooksOnSameRow := false
	for _, line := range lines {
		if strings.Contains(line, "DAEMON") && strings.Contains(line, "HOOKS") {
			// In wide mode they share a row via JoinHorizontal.
			hooksOnSameRow = true
			break
		}
	}
	// Check for HOOKS appearing on a line that also has content from the left column.
	// With two-column layout, HOOKS header appears on same row as DAEMON content.
	daemonLine := -1
	hooksLine := -1
	for i, line := range lines {
		if strings.Contains(line, "DAEMON") {
			daemonLine = i
		}
		if strings.Contains(line, "HOOKS") {
			hooksLine = i
		}
	}
	if !hooksOnSameRow && (daemonLine == -1 || hooksLine == -1 || hooksLine > daemonLine+1) {
		t.Error("at width 120, HOOKS should be in two-column layout near DAEMON")
	}
}

func TestModelViewNarrowLayout(t *testing.T) {
	m := newTestModel()
	m.SetSize(80, 40) // narrow mode
	view := ansi.Strip(m.View())

	// Should still have all sections, just stacked.
	if !strings.Contains(view, "DAEMON") {
		t.Error("narrow view missing DAEMON section")
	}
	if !strings.Contains(view, "HOOKS") {
		t.Error("narrow view missing HOOKS section")
	}
	if !strings.Contains(view, "STORE") {
		t.Error("narrow view missing STORE section")
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
