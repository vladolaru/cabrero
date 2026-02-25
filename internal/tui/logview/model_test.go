package logview

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
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
	// Title is now in SubHeader(), rendered by root model.
	subHeader := ansi.Strip(m.SubHeader())
	if !strings.Contains(subHeader, "Log Viewer") {
		t.Error("sub-header missing title")
	}
}

func TestLogModelSearch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Press / to activate search.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.searchActive {
		t.Error("search should be active after /")
	}

	// Type search term.
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Press Enter to search.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	m, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if m.followMode {
		t.Error("follow should be off after f")
	}

	// Press 'f' again.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if !m.followMode {
		t.Error("follow should be on after second f")
	}
}

func TestLogModelSearchHighlighting(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Activate search.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})

	// Type search term.
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}

	// Press Enter to search.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'classifier'")
	}

	// The highlighted content should contain ANSI escape sequences.
	highlighted := m.highlightedContent()
	if !strings.Contains(highlighted, "\x1b[") {
		t.Error("highlighted content should contain ANSI escape sequences")
	}

	// The highlighted content should differ from the raw content.
	if highlighted == strings.Join(m.lines, "\n") {
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
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !m.searchActive {
		t.Fatal("search should be active")
	}

	// Press Esc to close search.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !m.HasActiveSearch() {
		t.Error("should have active search after searching with matches")
	}
}

func TestLogModelTwoStageEsc(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Perform a search.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "classifier" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if !m.HasActiveSearch() {
		t.Fatal("should have active search after searching")
	}

	// First Esc: should clear matches, not propagate.
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
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
	// SetSize scrolls to the latest entry.
	if m.cursor != len(m.entries)-1 {
		t.Errorf("cursor should start at last entry (%d), got %d", len(m.entries)-1, m.cursor)
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

func TestViewShowsColoredLevels(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	view := m.View()

	// Should contain ANSI escape sequences (colored output).
	if !strings.Contains(view, "\x1b[") {
		t.Error("view should contain ANSI color codes for level badges")
	}

	// Stripped view should still have the content.
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "INFO") {
		t.Error("stripped view should contain INFO level")
	}
	if !strings.Contains(stripped, "daemon started") {
		t.Error("stripped view should contain message text")
	}
}

func TestViewShowsCursorIndicator(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	stripped := ansi.Strip(m.View())

	// First entry (cursor=0) should have ">" prefix.
	if !strings.Contains(stripped, ">") {
		t.Error("view should show cursor indicator '>'")
	}
}

func TestViewHasNoBlankLineSeparators(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Render entries directly (skip title/status lines from View).
	rendered := ansi.Strip(m.renderEntries())
	lines := strings.Split(strings.TrimRight(rendered, "\n"), "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			t.Errorf("unexpected blank line at position %d between entries", i)
		}
	}
}

func TestViewCollapsedMultiLineEntry(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	stripped := ansi.Strip(m.View())

	if !strings.Contains(stripped, "[+3]") {
		t.Error("collapsed multi-line entry should show [+3] indicator")
	}

	if strings.Contains(stripped, "config.Load") {
		t.Error("continuation lines should not be visible when entry is collapsed")
	}
}

func TestViewExpandedMultiLineEntry(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Expand the second entry (the ERROR with extra lines).
	m.cursor = 1
	m.entries[1].Expanded = true
	m.refreshViewportContent()

	stripped := ansi.Strip(m.View())

	if !strings.Contains(stripped, "config.Load") {
		t.Error("expanded entry should show continuation lines")
	}

	if !strings.Contains(stripped, "[-]") {
		t.Error("expanded entry should show [-] indicator")
	}
}

func TestCursorNavigation(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Cursor starts at the last entry after SetSize.
	last := len(m.entries) - 1
	if m.cursor != last {
		t.Fatalf("cursor should start at last entry (%d), got %d", last, m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != last-1 {
		t.Errorf("cursor should be %d after Up, got %d", last-1, m.cursor)
	}

	// Move down.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != last {
		t.Errorf("cursor should be %d after Down, got %d", last, m.cursor)
	}

	// Can't go below last.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.cursor != last {
		t.Errorf("cursor should stay at %d, got %d", last, m.cursor)
	}
}

func TestCursorNavigationDisablesFollow(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	if !m.followMode {
		t.Fatal("follow should start on")
	}

	// Moving cursor should disable follow mode.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.followMode {
		t.Error("cursor navigation should disable follow mode")
	}
}

func TestCursorStaysInBounds(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Move past the first entry.
	for i := 0; i < len(m.entries)+5; i++ {
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if m.cursor != 0 {
		t.Errorf("cursor should clamp to first entry (0), got %d", m.cursor)
	}
}

func TestToggleExpand(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Move cursor to the multi-line entry (index 1).
	// Cursor starts at last entry (2), so move up once.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.cursor != 1 {
		t.Fatalf("cursor should be 1, got %d", m.cursor)
	}

	// Press Enter to expand.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.entries[1].Expanded {
		t.Error("entry should be expanded after Enter")
	}

	// View should now show continuation lines.
	stripped := ansi.Strip(m.View())
	if !strings.Contains(stripped, "config.Load") {
		t.Error("expanded view should show continuation lines")
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.entries[1].Expanded {
		t.Error("entry should be collapsed after second Enter")
	}
}

func TestToggleExpandSingleLine(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// All entries are single-line in testLogContent.
	// Enter on single-line entry should be a no-op.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// No crash, no change.
}

func TestExpandAll(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Press 'e' to expand all.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	for i, entry := range m.entries {
		if entry.IsMultiLine() && !entry.Expanded {
			t.Errorf("entry[%d] should be expanded after 'e'", i)
		}
	}
}

func TestCollapseAll(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Expand all first.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})

	// Press 'E' to collapse all.
	m, _ = m.Update(tea.KeyPressMsg{Code: 'E', Text: "E"})
	for i, entry := range m.entries {
		if entry.Expanded {
			t.Errorf("entry[%d] should be collapsed after 'E'", i)
		}
	}
}

func TestSearchExpandsAllMultiLineEntries(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Search for text that's in a continuation line.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "config.Load" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'config.Load'")
	}

	// All multi-line entries should be auto-expanded during search.
	for i, entry := range m.entries {
		if entry.IsMultiLine() && !entry.Expanded {
			t.Errorf("entry[%d] should be expanded during search", i)
		}
	}
}

func TestLogModelView_SearchMatchBarFitsTerminalWidth(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(80, 20)

	// Simulate a completed search with matches.
	m.searchActive = false
	m.searchTerm = "daemon"
	m.matches = []lineMatch{{entryIdx: 0}, {entryIdx: 2}, {entryIdx: 4}}
	m.matchIdx = 0

	view := m.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("View() returned empty string")
	}
	last := lines[len(lines)-1]

	// Status bar must fit within terminal width.
	width := ansi.StringWidth(last)
	if width > 80 {
		t.Errorf("status bar width = %d, want ≤ 80\ngot: %q", width, last)
	}

	// Match count must be visible in the bar.
	stripped := ansi.Strip(last)
	if !strings.Contains(stripped, "1/3 matches") {
		t.Errorf("status bar missing match count\ngot: %q", stripped)
	}
}

func TestSearchJumpsToLastMatch(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Search for "session" which appears in multiple entries.
	m, _ = m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range "session" {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'session'")
	}

	// Cursor should be at the last matching entry.
	lastMatch := m.matches[len(m.matches)-1]
	if m.cursor != lastMatch.entryIdx {
		t.Errorf("cursor = %d, want %d (last match entry)", m.cursor, lastMatch.entryIdx)
	}
	if m.matchIdx != len(m.matches)-1 {
		t.Errorf("matchIdx = %d, want %d (last match)", m.matchIdx, len(m.matches)-1)
	}
}

func TestFollowTick_DetectsNewBytes(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	initial := "line1\nline2\n"
	if err := os.WriteFile(logPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(initial)))

	appended := "line3\n"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(appended)
	f.Close()

	cmd := m.FollowTick(logPath)
	msg := cmd()

	result, ok := msg.(LogAppended)
	if !ok {
		t.Fatalf("expected LogAppended, got %T", msg)
	}
	if result.NewContent != appended {
		t.Errorf("NewContent = %q, want %q", result.NewContent, appended)
	}
	want := int64(len(initial) + len(appended))
	if result.NewFileSize != want {
		t.Errorf("NewFileSize = %d, want %d", result.NewFileSize, want)
	}
}

func TestFollowTick_DetectsRotation(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	initial := "old line1\nold line2\n"
	if err := os.WriteFile(logPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(initial)))

	rotated := "new\n"
	if err := os.WriteFile(logPath, []byte(rotated), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := m.FollowTick(logPath)
	msg := cmd()

	result, ok := msg.(LogReplaced)
	if !ok {
		t.Fatalf("expected LogReplaced, got %T", msg)
	}
	if result.Content != rotated {
		t.Errorf("Content = %q, want %q", result.Content, rotated)
	}
}

func TestFollowTick_NoChangeReturnsNil(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "daemon.log")
	content := "line1\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newTestLogModel()
	m.SetFileSize(int64(len(content)))

	cmd := m.FollowTick(logPath)
	msg := cmd()

	if msg != nil {
		t.Errorf("expected nil msg for unchanged file, got %T: %v", msg, msg)
	}
}
