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
	m.SetSize(70, 40) // narrow mode (< 80)
	view := ansi.Strip(m.View())

	// Essentials should be present.
	if !strings.Contains(view, "DAEMON") {
		t.Error("narrow view missing DAEMON section")
	}
	if !strings.Contains(view, "HOOKS") {
		t.Error("narrow view missing HOOKS section")
	}

	// Abbreviated: no intervals, no STORE section.
	if strings.Contains(view, "Poll:") {
		t.Error("narrow view should not show Poll interval")
	}
	if strings.Contains(view, "STORE") {
		t.Error("narrow view should not show STORE section")
	}

	// Prompts should be hidden in narrow mode.
	if strings.Contains(view, "PROMPTS") {
		t.Error("narrow view should not show PROMPTS section")
	}
}

func TestModelViewStandardLayout(t *testing.T) {
	m := newTestModel()
	m.SetSize(100, 40) // standard mode (80-119)
	view := ansi.Strip(m.View())

	// All sections present, stacked (not side-by-side).
	if !strings.Contains(view, "DAEMON") {
		t.Error("standard view missing DAEMON section")
	}
	if !strings.Contains(view, "STORE") {
		t.Error("standard view missing STORE section")
	}
	if !strings.Contains(view, "PROMPTS") {
		t.Error("standard view missing PROMPTS section")
	}

	// Stacked: DAEMON and HOOKS should NOT share a line.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "DAEMON") && strings.Contains(line, "HOOKS") {
			t.Error("standard view should stack DAEMON and HOOKS, not side-by-side")
		}
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

func TestModelViewRunRowWide(t *testing.T) {
	m := newTestModel()
	m.SetSize(140, 40)
	view := ansi.Strip(m.View())

	// Wide: 8-char ID, full project name (up to 20), all 3 timing stages.
	if !strings.Contains(view, "e7f2a103") {
		t.Error("wide view should show 8-char session ID")
	}
	// All 3 timing columns.
	if !strings.Contains(view, "parse") || !strings.Contains(view, "cls") || !strings.Contains(view, "eval") {
		t.Error("wide view should show all 3 timing stages")
	}
}

func TestModelViewRunRowStandard(t *testing.T) {
	m := newTestModel()
	m.SetSize(100, 40)
	view := ansi.Strip(m.View())

	// Standard: 8-char ID still shown.
	if !strings.Contains(view, "e7f2a103") {
		t.Error("standard view should show 8-char session ID")
	}
	// Standard: 2 stages (parse + eval) — classifier omitted.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "e7f2a103") {
			if strings.Contains(line, "cls") {
				t.Error("standard view should omit classifier timing (show 2 stages)")
			}
			break
		}
	}
}

func TestModelViewRunRowNarrow(t *testing.T) {
	m := newTestModel()
	m.SetSize(70, 40)
	view := ansi.Strip(m.View())

	// Narrow: 8-char short session ID.
	if !strings.Contains(view, "e7f2a103") {
		t.Error("narrow view should show 8-char session ID")
	}
	// Narrow: total-only timing (no stage names).
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "e7f2a103") {
			if strings.Contains(line, "parse") || strings.Contains(line, "cls") || strings.Contains(line, "eval") {
				t.Error("narrow view should show total timing only, not per-stage")
			}
			break
		}
	}
}

func TestModelViewNarrowActivityStats(t *testing.T) {
	m := newTestModel()
	m.SetSize(70, 40) // narrow
	view := ansi.Strip(m.View())

	// Activity section should still be present.
	if !strings.Contains(view, "PIPELINE ACTIVITY") {
		t.Error("narrow view missing PIPELINE ACTIVITY section")
	}

	// Sparkline should be hidden in narrow mode.
	if strings.Contains(view, "sessions/day") {
		t.Error("narrow view should not show sparkline")
	}
}

func TestLayoutMode(t *testing.T) {
	m := newTestModel()

	m.SetSize(120, 40)
	if m.layoutMode() != layoutWide {
		t.Errorf("width 120 should be wide, got %d", m.layoutMode())
	}

	m.SetSize(100, 40)
	if m.layoutMode() != layoutStandard {
		t.Errorf("width 100 should be standard, got %d", m.layoutMode())
	}

	m.SetSize(79, 40)
	if m.layoutMode() != layoutNarrow {
		t.Errorf("width 79 should be narrow, got %d", m.layoutMode())
	}

	// Boundary: 80 is standard, not narrow.
	m.SetSize(80, 40)
	if m.layoutMode() != layoutStandard {
		t.Errorf("width 80 should be standard, got %d", m.layoutMode())
	}
}

func TestPipeline_Viewport_ExistsAfterSetSize(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 24)

	// Viewport height = 24 - 1 (status bar).
	if m.viewport.Height != 23 {
		t.Fatalf("viewport.Height = %d, want 23", m.viewport.Height)
	}
	if m.viewport.Width != 120 {
		t.Fatalf("viewport.Width = %d, want 120", m.viewport.Width)
	}
}

func TestPipeline_Viewport_ContentRendered(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "DAEMON") {
		t.Error("View should contain DAEMON section")
	}
	if !strings.Contains(view, "PIPELINE ACTIVITY") {
		t.Error("View should contain PIPELINE ACTIVITY section")
	}
	if !strings.Contains(view, "RECENT RUNS") {
		t.Error("View should contain RECENT RUNS section")
	}
}

func TestModelPipelineKeyEmitsPushLogView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Press L.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
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
