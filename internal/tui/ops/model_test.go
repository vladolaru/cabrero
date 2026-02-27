package ops

import (
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

func newTestModel() Model {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	stats := testdata.TestOpsStats()
	return New(stats, &keys, cfg)
}

func TestNewModel(t *testing.T) {
	m := newTestModel()
	if m.cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.cursor)
	}
	if m.stats.GatedRuns != 4 {
		t.Errorf("GatedRuns = %d, want 4", m.stats.GatedRuns)
	}
}

func TestModelView_Empty(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(pipeline.OpsStats{}, &keys, cfg)
	m.SetSize(120, 30)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "No operational events") {
		t.Error("empty view should show empty state message")
	}
}

func TestModelView_WithData(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	view := ansi.Strip(m.View())

	if !strings.Contains(view, "Summary") {
		t.Error("view missing Summary section")
	}
	if !strings.Contains(view, "Gated runs") {
		t.Error("view missing Gated runs card")
	}
	if !strings.Contains(view, "Skipped (busy)") {
		t.Error("view missing Skipped (busy) card")
	}
	if !strings.Contains(view, "Meta Analysis") {
		t.Error("view missing Meta Analysis section")
	}
	if !strings.Contains(view, "Gate Breakdown") {
		t.Error("view missing Gate Breakdown section")
	}
	if !strings.Contains(view, "Recent Events") {
		t.Error("view missing Recent Events section")
	}
}

func TestModelSubHeader(t *testing.T) {
	m := newTestModel()
	sub := ansi.Strip(m.SubHeader())

	if !strings.Contains(sub, "Operations") {
		t.Error("sub-header missing title")
	}
	if !strings.Contains(sub, "operational event") {
		t.Error("sub-header missing event count")
	}
}

func TestModelNavigation_Down(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor after down = %d, want 1", m.cursor)
	}
}

func TestModelNavigation_Up(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Move down then up.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor after up = %d, want 0", m.cursor)
	}
}

func TestModelNavigation_BoundsDown(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	eventCount := len(m.stats.RecentEvents)
	// Move down past the end.
	for i := 0; i < eventCount+5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.cursor != eventCount-1 {
		t.Errorf("cursor should clamp at %d, got %d", eventCount-1, m.cursor)
	}
}

func TestModelNavigation_BoundsUp(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	// Move up from start.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", m.cursor)
	}
}

func TestModelBackEmitsPopView(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("Esc should produce cmd")
	}
	msg := cmd()
	if _, ok := msg.(message.PopView); !ok {
		t.Errorf("expected PopView, got %T", msg)
	}
}

func TestModelUpdateStats(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	newStats := pipeline.OpsStats{
		GatedRuns:   10,
		SkippedBusy: 20,
		ErrorRuns:   3,
	}
	m.UpdateStats(newStats)

	if m.stats.GatedRuns != 10 {
		t.Errorf("GatedRuns after update = %d, want 10", m.stats.GatedRuns)
	}
	if m.stats.SkippedBusy != 20 {
		t.Errorf("SkippedBusy after update = %d, want 20", m.stats.SkippedBusy)
	}
}

func TestModelOpsDataRefreshed(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	newStats := pipeline.OpsStats{
		GatedRuns:   15,
		SkippedBusy: 25,
	}
	m, _ = m.Update(message.OpsDataRefreshed{Stats: newStats})

	if m.stats.GatedRuns != 15 {
		t.Errorf("GatedRuns after refresh = %d, want 15", m.stats.GatedRuns)
	}
}

func TestModelHasActivePrompt(t *testing.T) {
	m := newTestModel()
	if m.HasActivePrompt() {
		t.Error("ops view should never have active prompt")
	}
}

func TestModelResize(t *testing.T) {
	m := newTestModel()
	m.SetSize(120, 40)

	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	// Verify view still renders without panic.
	view := ansi.Strip(m.View())
	if view == "" {
		t.Error("view should not be empty after resize")
	}
}

func TestModelViewZeroSize(t *testing.T) {
	m := newTestModel()
	// Don't call SetSize.
	view := m.View()
	if view != "" {
		t.Errorf("view at zero size should be empty, got %q", view)
	}
}

func TestRenderEvent_AllStatuses(t *testing.T) {
	now := time.Now()
	events := []pipeline.OpsEvent{
		{Timestamp: now, SessionID: "sess1", Source: "daemon", Status: pipeline.HistoryStatusSkippedBusy},
		{Timestamp: now, SessionID: "sess2", Source: "daemon", Status: pipeline.HistoryStatusError, Reason: "timeout"},
		{Timestamp: now, SessionID: "sess3", Source: "daemon", Status: pipeline.HistoryStatusProcessed},
		{Timestamp: now, Source: "meta", Status: pipeline.HistoryStatusMetaTriggered, Reason: "v3"},
		{Timestamp: now, Source: "meta", Status: pipeline.HistoryStatusMetaCooldown, Reason: "v2"},
	}

	for _, ev := range events {
		rendered := ansi.Strip(renderEvent(ev))
		if rendered == "" {
			t.Errorf("renderEvent for status %q produced empty output", ev.Status)
		}
	}
}
