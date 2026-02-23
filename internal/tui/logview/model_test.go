package logview

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vladolaru/cabrero/internal/tui/shared"
	"github.com/vladolaru/cabrero/internal/tui/testdata"
)

// testLogContent uses the real daemon log format produced by internal/daemon/log.go:63
// Format: "2006-01-02T15:04:05 [LEVEL] message"
var testLogContent = "2026-02-20T10:15:03 [INFO] daemon started (PID 4821)\n" +
	"2026-02-20T10:15:03 [INFO] poll=2m0s stale=30m0s delay=30s\n" +
	"2026-02-20T10:15:03 [INFO] processing session e7f2a103\n" +
	"2026-02-20T10:15:04 [INFO] pre-parser: 142 entries, 0.8s\n" +
	"2026-02-20T10:15:12 [INFO] classifier: classified, triage=evaluate\n" +
	"2026-02-20T10:15:24 [INFO] evaluator: 1 proposal generated\n" +
	"2026-02-20T10:17:05 [INFO] poll: 0 pending sessions\n"

// testLogMultiLine contains multi-line entries (e.g. stack traces) for testing.
var testLogMultiLine = "2026-02-20T10:15:03 [INFO] daemon started (PID 4821)\n" +
	"2026-02-20T10:15:04 [ERROR] failed to read config\n" +
	"  at config.Load (config.go:42)\n" +
	"  at main.init (main.go:15)\n" +
	"  caused by: file not found\n" +
	"2026-02-20T10:15:05 [INFO] pipeline run completed\n"

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

func TestParseEntries(t *testing.T) {
	entries := parseEntries(testLogContent)
	if len(entries) != 7 {
		t.Fatalf("expected 7 entries, got %d", len(entries))
	}
	if entries[0].Level != "INFO" {
		t.Errorf("entry[0].Level = %q, want INFO", entries[0].Level)
	}
	if entries[0].Message != "daemon started (PID 4821)" {
		t.Errorf("entry[0].Message = %q, want 'daemon started (PID 4821)'", entries[0].Message)
	}
	if entries[0].Timestamp != "2026-02-20T10:15:03" {
		t.Errorf("entry[0].Timestamp = %q", entries[0].Timestamp)
	}
}

func TestParseEntriesMultiLine(t *testing.T) {
	entries := parseEntries(testLogMultiLine)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	e := entries[1]
	if e.Level != "ERROR" {
		t.Errorf("entry[1].Level = %q, want ERROR", e.Level)
	}
	if e.Message != "failed to read config" {
		t.Errorf("entry[1].Message = %q", e.Message)
	}
	if len(e.Extra) != 3 {
		t.Fatalf("entry[1].Extra len = %d, want 3", len(e.Extra))
	}
	if e.Extra[0] != "  at config.Load (config.go:42)" {
		t.Errorf("entry[1].Extra[0] = %q", e.Extra[0])
	}

	if len(entries[2].Extra) != 0 {
		t.Errorf("entry[2] should have no extra lines, got %d", len(entries[2].Extra))
	}
}

func TestParseEntriesEmpty(t *testing.T) {
	entries := parseEntries("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(entries))
	}
}

func TestParseEntriesUnparseable(t *testing.T) {
	entries := parseEntries("some random text\nanother line\n")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for unparseable content, got %d", len(entries))
	}
	if entries[0].Level != "" {
		t.Errorf("unparseable entry should have empty level, got %q", entries[0].Level)
	}
}

func TestModelHasEntries(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	if len(m.entries) == 0 {
		t.Fatal("model should parse content into entries")
	}
	if m.cursor != 0 {
		t.Errorf("cursor should start at 0, got %d", m.cursor)
	}
}

func TestModelMultiLineEntries(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	if len(m.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(m.entries))
	}
	if !m.entries[1].IsMultiLine() {
		t.Error("entry[1] should be multi-line")
	}
	if m.entries[1].Expanded {
		t.Error("multi-line entries should be collapsed by default")
	}
}

func TestModelUpdateContentReParsesEntries(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	originalCount := len(m.entries)
	if originalCount == 0 {
		t.Fatal("model should start with entries")
	}

	// Update with different content.
	m.UpdateContent(testLogMultiLine)
	if len(m.entries) != 3 {
		t.Fatalf("after UpdateContent, expected 3 entries, got %d", len(m.entries))
	}
}

func TestModelAppendContentReParsesEntries(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	originalCount := len(m.entries)
	m.AppendContent("2026-02-20T10:20:00 [WARN] disk space low\n")
	if len(m.entries) != originalCount+1 {
		t.Fatalf("after AppendContent, expected %d entries, got %d", originalCount+1, len(m.entries))
	}
}

func TestModelCursorClamped(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Set cursor past the end.
	m.cursor = 999
	m.clampCursor()
	if m.cursor != len(m.entries)-1 {
		t.Errorf("cursor should be clamped to %d, got %d", len(m.entries)-1, m.cursor)
	}

	// Set cursor negative.
	m.cursor = -5
	m.clampCursor()
	if m.cursor != 0 {
		t.Errorf("cursor should be clamped to 0, got %d", m.cursor)
	}
}

func TestModelFollowModeMovesToLastEntry(t *testing.T) {
	cfg := testdata.TestConfig()
	cfg.Pipeline.LogFollowMode = true
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogContent, &keys, cfg)
	m.SetSize(120, 40)

	// In follow mode, UpdateContent should move cursor to last entry.
	m.UpdateContent(testLogMultiLine)
	if m.cursor != len(m.entries)-1 {
		t.Errorf("follow mode: cursor should be %d, got %d", len(m.entries)-1, m.cursor)
	}

	// AppendContent should also move cursor to last entry.
	m.AppendContent("2026-02-20T10:20:00 [WARN] disk space low\n")
	if m.cursor != len(m.entries)-1 {
		t.Errorf("follow mode after append: cursor should be %d, got %d", len(m.entries)-1, m.cursor)
	}
}

func TestModelEmptyContentHasNoEntries(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New("", &keys, cfg)
	m.SetSize(120, 40)

	if len(m.entries) != 0 {
		t.Errorf("empty content should have 0 entries, got %d", len(m.entries))
	}
	if m.cursor != 0 {
		t.Errorf("cursor should be 0 for empty content, got %d", m.cursor)
	}
}
