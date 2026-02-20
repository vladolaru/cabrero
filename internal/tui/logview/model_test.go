package logview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

var testLogContent = `2026-02-20T10:15:03Z INFO  daemon started (PID 4821)
2026-02-20T10:15:03Z INFO  poll=2m0s stale=30m0s delay=30s
2026-02-20T10:15:03Z INFO  processing session e7f2a103
2026-02-20T10:15:04Z INFO  pre-parser: 142 entries, 0.8s
2026-02-20T10:15:12Z INFO  classifier: classified, triage=evaluate
2026-02-20T10:15:24Z INFO  evaluator: 1 proposal generated
2026-02-20T10:17:05Z INFO  poll: 0 pending sessions
`

func newTestLogModel() Model {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	return New(testLogContent, &keys, cfg)
}

func TestNewLogModel(t *testing.T) {
	m := newTestLogModel()
	if !m.followMode {
		t.Error("follow mode should be on by default")
	}
}

func TestLogModelView(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	view := ansi.Strip(m.View())

	if !strings.Contains(view, "daemon started") {
		t.Error("view missing log content")
	}
	if !strings.Contains(view, "Log Viewer") {
		t.Error("view missing title")
	}
}

func TestLogModelSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Press / to activate search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searchActive {
		t.Error("search should be active after /")
	}

	// Type search term.
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.matches) == 0 {
		t.Error("expected matches for 'classifier'")
	}
}

func TestLogModelFollowToggle(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	if !m.followMode {
		t.Fatal("follow should start on")
	}

	// Press 'f' to toggle.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.followMode {
		t.Error("follow should be off after f")
	}

	// Press 'f' again.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !m.followMode {
		t.Error("follow should be on after second f")
	}
}

func TestLogModelSearchHighlighting(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Activate search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})

	// Type search term.
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press Enter to search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'classifier'")
	}

	// The highlighted content should contain ANSI escape sequences.
	highlighted := m.highlightedContent()
	if !strings.Contains(highlighted, "\x1b[") {
		t.Error("highlighted content should contain ANSI escape sequences")
	}

	// The highlighted content should differ from the raw content.
	if highlighted == m.content {
		t.Error("highlighted content should differ from raw content when matches exist")
	}

	// The stripped highlighted content should still contain the match text.
	stripped := ansi.Strip(highlighted)
	if !strings.Contains(stripped, "classifier") {
		t.Error("stripped highlighted content should still contain the match text")
	}

	// Non-matching lines should be unchanged.
	if !strings.Contains(stripped, "daemon started") {
		t.Error("non-matching content should be preserved")
	}
}

func TestLogModelEscFromSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Activate search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.searchActive {
		t.Fatal("search should be active")
	}

	// Press Esc to close search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searchActive {
		t.Error("search should be closed after Esc")
	}
}

func TestLogModelHasActiveSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// No search yet.
	if m.HasActiveSearch() {
		t.Error("should not have active search initially")
	}

	// Perform search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.HasActiveSearch() {
		t.Error("should have active search after searching with matches")
	}
}

func TestLogModelTwoStageEsc(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Perform a search.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !m.HasActiveSearch() {
		t.Fatal("should have active search after searching")
	}

	// First Esc: should clear matches, not propagate.
	m, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.HasActiveSearch() {
		t.Error("first Esc should clear active search")
	}
	if cmd != nil {
		t.Error("first Esc should not produce a command (handled locally)")
	}
	// Search term and matches should be cleared.
	if m.searchTerm != "" {
		t.Errorf("searchTerm should be empty after Esc, got %q", m.searchTerm)
	}
	if len(m.matches) != 0 {
		t.Errorf("matches should be empty after Esc, got %d", len(m.matches))
	}

	// Second Esc: log viewer has no matches, so it should NOT handle Esc.
	// (The root model's global key handler will produce PopView.)
	// Since we're testing the logview in isolation, Esc just falls through to viewport.
}
