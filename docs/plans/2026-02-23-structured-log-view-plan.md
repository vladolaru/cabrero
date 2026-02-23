# Structured Log View Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace the plain-text log viewer with a structured, colorized, collapsible view that parses entries, colors level badges, adds visual separators, and supports cursor-based expand/collapse navigation.

**Architecture:** Parse raw log lines into `LogEntry` structs (timestamp + level + message + extra lines). Render entries into a viewport with colored level badges, cursor indicator, and blank-line separators. Dual navigation: cursor moves between entries (j/k), viewport scrolls freely (PgUp/PgDn). Expand/collapse multi-line entries via Enter.

**Tech Stack:** Go, Bubble Tea (viewport, textinput), lipgloss (styling), termenv (ANSI highlighting), regexp (log line parsing)

**Design doc:** `docs/plans/2026-02-23-structured-log-view-design.md`

---

## Important Context

### Log Format Mismatch

The daemon (`internal/daemon/log.go:63`) writes:
```
2026-02-20T10:15:03 [INFO] daemon started (PID 4821)
```

But existing test data (`internal/tui/logview/model_test.go:14-21`) uses a different format:
```
2026-02-20T10:15:03Z INFO  daemon started (PID 4821)
```

The parser must handle the real daemon format (`[LEVEL]` with brackets). Update the test data to match the real format.

### Existing Patterns to Follow

- **Viewport + cursor index:** See `internal/tui/fitness/model.go` — `selectedEvidence int` tracks cursor within viewport content. On cursor change, re-render viewport content and use `viewport.SetYOffset()` to keep cursor visible.
- **Expand/collapse toggle:** See `internal/tui/fitness/update.go:115-126` — toggle `Expanded` bool, re-render viewport content.
- **Color styles:** All defined in `internal/tui/shared/styles.go` — `AccentStyle`, `ErrorStyle`, `MutedStyle`, etc. Adaptive for light/dark terminals.
- **Flat item pattern:** See `internal/tui/sources/model.go:29-34` — `flatItem` maps visible rows to data indices. We'll use a simpler `entryLineMap` that tracks which rendered line belongs to which entry index.

### Key Bindings

- Existing log viewer bindings in `internal/tui/shared/keys.go:109-113`: `Search`, `SearchNext`, `SearchPrev`, `FollowToggle`
- Navigation bindings (`Up`/`Down`/`Open`/etc.) are shared across views
- New bindings needed: `ExpandAll` (`e`) and `CollapseAll` (`E`)

---

### Task 1: Add LogEntry Type and Parser

**Files:**
- Modify: `internal/tui/logview/model.go`
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write parsing tests**

Add to `model_test.go`. First, fix the test log content to match the real daemon format, then add parsing tests.

```go
// Replace the existing testLogContent with real daemon format.
var testLogContent = "2026-02-20T10:15:03 [INFO] daemon started (PID 4821)\n" +
	"2026-02-20T10:15:03 [INFO] poll=2m0s stale=30m0s delay=30s\n" +
	"2026-02-20T10:15:03 [INFO] processing session e7f2a103\n" +
	"2026-02-20T10:15:04 [INFO] pre-parser: 142 entries, 0.8s\n" +
	"2026-02-20T10:15:12 [INFO] classifier: classified, triage=evaluate\n" +
	"2026-02-20T10:15:24 [INFO] evaluator: 1 proposal generated\n" +
	"2026-02-20T10:17:05 [INFO] poll: 0 pending sessions\n"

// Test content with multi-line entries (stack traces).
var testLogMultiLine = "2026-02-20T10:15:03 [INFO] daemon started (PID 4821)\n" +
	"2026-02-20T10:15:04 [ERROR] failed to read config\n" +
	"  at config.Load (config.go:42)\n" +
	"  at main.init (main.go:15)\n" +
	"  caused by: file not found\n" +
	"2026-02-20T10:15:05 [INFO] pipeline run completed\n"

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

	// Second entry should have extra lines.
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

	// Third entry should be single-line.
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
	// Lines that don't match the log format should still appear as entries.
	entries := parseEntries("some random text\nanother line\n")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for unparseable content, got %d", len(entries))
	}
	if entries[0].Level != "" {
		t.Errorf("unparseable entry should have empty level, got %q", entries[0].Level)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run TestParseEntries -v`
Expected: FAIL — `parseEntries` function doesn't exist yet.

**Step 3: Implement LogEntry type and parseEntries**

Add to `model.go`:

```go
import "regexp"

// logLineRe matches daemon log lines: "2026-02-20T10:15:03 [INFO] message"
var logLineRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\s+\[(\w+)\]\s+(.*)`)

// LogEntry represents a parsed log line with optional continuation lines.
type LogEntry struct {
	Timestamp string   // "2026-02-20T10:15:03"
	Level     string   // "INFO", "ERROR"
	Message   string   // first line of the message
	Extra     []string // continuation lines (stack traces, JSON, etc.)
	Expanded  bool     // whether Extra is visible (default: false)
}

// IsMultiLine reports whether this entry has continuation lines.
func (e LogEntry) IsMultiLine() bool {
	return len(e.Extra) > 0
}

// parseEntries splits raw log content into structured entries.
// Lines matching the log format start new entries; other lines are
// continuation lines appended to the current entry's Extra.
func parseEntries(content string) []LogEntry {
	if content == "" {
		return nil
	}
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) == 0 {
		return nil
	}

	var entries []LogEntry
	for _, line := range lines {
		if m := logLineRe.FindStringSubmatch(line); m != nil {
			entries = append(entries, LogEntry{
				Timestamp: m[1],
				Level:     m[2],
				Message:   m[3],
			})
		} else if len(entries) > 0 {
			// Continuation line — append to current entry.
			entries[len(entries)-1].Extra = append(entries[len(entries)-1].Extra, line)
		} else {
			// Leading unparseable line — create a raw entry.
			entries = append(entries, LogEntry{Message: line})
		}
	}
	return entries
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -run TestParseEntries -v`
Expected: PASS (all 4 tests)

**Step 5: Commit**

```
feat(tui): add LogEntry type and log line parser

Parses raw daemon log lines into structured LogEntry values with
timestamp, level, message, and continuation lines (for stack traces).
Handles both single-line and multi-line entries.
```

---

### Task 2: Wire Entries Into Model

**Files:**
- Modify: `internal/tui/logview/model.go`
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write tests for model with entries and cursor**

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run TestModel -v`
Expected: FAIL — `m.entries` field doesn't exist yet.

**Step 3: Add entries and cursor to Model**

In `model.go`, modify the `Model` struct and `New` function:

```go
type Model struct {
	content      string
	lines        []string
	entries      []LogEntry // parsed structured entries
	cursor       int        // selected entry index
	viewport     viewport.Model
	searchInput  textinput.Model
	searchActive bool
	searchTerm   string
	followMode   bool
	matches      []lineMatch
	matchIdx     int
	width        int
	height       int
	keys         *shared.KeyMap
	config       *shared.Config
}

func New(content string, keys *shared.KeyMap, cfg *shared.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	lines := strings.Split(content, "\n")
	entries := parseEntries(content)

	m := Model{
		content:     content,
		lines:       lines,
		entries:     entries,
		followMode:  cfg.Pipeline.LogFollowMode,
		matchIdx:    -1,
		keys:        keys,
		config:      cfg,
		searchInput: ti,
	}
	return m
}
```

Also update `UpdateContent` and `AppendContent` to re-parse entries:

```go
func (m *Model) UpdateContent(content string) {
	m.content = content
	m.lines = strings.Split(content, "\n")
	m.entries = parseEntries(content)
	m.clampCursor()
	m.refreshViewportContent()
	if m.followMode {
		m.cursor = max(0, len(m.entries)-1)
		m.viewport.GotoBottom()
	}
}

func (m *Model) AppendContent(newBytes string) {
	if newBytes == "" {
		return
	}
	m.content += newBytes

	// Re-split lines incrementally (existing logic).
	newLines := strings.Split(newBytes, "\n")
	if len(m.lines) > 0 && len(newLines) > 0 {
		m.lines[len(m.lines)-1] += newLines[0]
		if len(newLines) > 1 {
			m.lines = append(m.lines, newLines[1:]...)
		}
	} else {
		m.lines = append(m.lines, newLines...)
	}

	// Re-parse entries from full content.
	// (For now, full re-parse is acceptable — optimise later if profiling shows need.)
	m.entries = parseEntries(m.content)

	m.refreshViewportContent()
	if m.followMode {
		m.cursor = max(0, len(m.entries)-1)
		m.viewport.GotoBottom()
	}
}

// clampCursor ensures cursor stays within valid range.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -v`
Expected: PASS (all tests including existing ones)

**Step 5: Commit**

```
feat(tui): wire parsed entries and cursor into log viewer model

Model now parses content into LogEntry slices and tracks a cursor.
UpdateContent and AppendContent re-parse entries. Cursor is clamped
to valid range. Follow mode moves cursor to the last entry.
```

---

### Task 3: Render Entries With Colors and Separators

**Files:**
- Modify: `internal/tui/logview/view.go`
- Modify: `internal/tui/logview/model.go` (add `renderEntries` method)
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write rendering tests**

```go
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

func TestViewShowsBlankLineSeparators(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)
	stripped := ansi.Strip(m.View())

	// Should have blank lines between entries (double newlines in output).
	// The viewport content (between title and status bar) should have them.
	lines := strings.Split(stripped, "\n")
	hasBlankLine := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			hasBlankLine = true
			break
		}
	}
	if !hasBlankLine {
		t.Error("view should contain blank lines between entries")
	}
}

func TestViewCollapsedMultiLineEntry(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	stripped := ansi.Strip(m.View())

	// Collapsed multi-line entry should show [+N lines] indicator.
	if !strings.Contains(stripped, "[+3]") {
		t.Error("collapsed multi-line entry should show [+3] indicator")
	}

	// Continuation lines should NOT be visible when collapsed.
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

	// Should show continuation lines.
	if !strings.Contains(stripped, "config.Load") {
		t.Error("expanded entry should show continuation lines")
	}

	// Should show collapse indicator [-].
	if !strings.Contains(stripped, "[-]") {
		t.Error("expanded entry should show [-] indicator")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run "TestView(Shows|Collapsed|Expanded)" -v`
Expected: FAIL — rendering not yet updated.

**Step 3: Implement colored entry rendering**

In `model.go`, add `renderEntries()` which replaces `highlightedContent()` for the viewport:

```go
// renderEntries builds the styled viewport content from parsed entries.
func (m *Model) renderEntries() string {
	if len(m.entries) == 0 {
		return m.content // fallback to raw content
	}

	var b strings.Builder
	for i, entry := range m.entries {
		if i > 0 {
			b.WriteString("\n") // blank line between entries
		}

		// Cursor prefix.
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}

		// Timestamp (muted).
		ts := ""
		if entry.Timestamp != "" {
			ts = highlightOutput.String(entry.Timestamp).
				Foreground(highlightOutput.Color(mutedColor())).String() + "  "
		}

		// Level badge (colored).
		lvl := ""
		if entry.Level != "" {
			lvl = renderLevel(entry.Level) + "  "
		}

		// First line: prefix + timestamp + level + message.
		b.WriteString(prefix)
		b.WriteString(ts)
		b.WriteString(lvl)
		b.WriteString(entry.Message)

		// Multi-line indicator.
		if entry.IsMultiLine() {
			if entry.Expanded {
				b.WriteString(" ")
				b.WriteString(highlightOutput.String("[-]").
					Foreground(highlightOutput.Color(mutedColor())).String())
			} else {
				b.WriteString(" ")
				indicator := fmt.Sprintf("[+%d]", len(entry.Extra))
				b.WriteString(highlightOutput.String(indicator).
					Foreground(highlightOutput.Color(mutedColor())).String())
			}
		}
		b.WriteString("\n")

		// Extra lines (only when expanded).
		if entry.Expanded && entry.IsMultiLine() {
			indent := "  " // align with message (after prefix)
			for _, extra := range entry.Extra {
				b.WriteString(indent)
				b.WriteString(extra)
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// renderLevel returns a colored level badge.
func renderLevel(level string) string {
	var color string
	switch level {
	case "ERROR":
		color = errorColor()
	default:
		color = accentColor()
	}
	return highlightOutput.String(level).Foreground(highlightOutput.Color(color)).String()
}

// Color helper functions that read the adaptive colors.
func mutedColor() string {
	if lipgloss.HasDarkBackground() {
		return shared.ColorMuted.Dark
	}
	return shared.ColorMuted.Light
}

func errorColor() string {
	if lipgloss.HasDarkBackground() {
		return shared.ColorError.Dark
	}
	return shared.ColorError.Light
}

func accentColor() string {
	if lipgloss.HasDarkBackground() {
		return shared.ColorAccent.Dark
	}
	return shared.ColorAccent.Light
}
```

Update `refreshViewportContent()` to use `renderEntries()`:

```go
func (m *Model) refreshViewportContent() {
	if len(m.entries) > 0 {
		content := m.renderEntries()
		if m.searchTerm != "" && len(m.matches) > 0 {
			content = m.applySearchHighlights(content)
		}
		m.viewport.SetContent(content)
	} else {
		m.viewport.SetContent(m.highlightedContent())
	}
}
```

Note: `applySearchHighlights` will be updated in Task 5. For now, the search highlight path can remain using the existing `highlightedContent()` as a fallback. A simple initial version of `applySearchHighlights` can just apply the same term-based highlighting to the already-rendered content:

```go
// applySearchHighlights overlays search match highlighting onto rendered content.
func (m *Model) applySearchHighlights(content string) string {
	if m.searchTerm == "" {
		return content
	}
	term := strings.ToLower(m.searchTerm)
	termLen := len(m.searchTerm)

	// Split on newlines, highlight matching portions, rejoin.
	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		stripped := ansi.Strip(line)
		lower := strings.ToLower(stripped)
		if strings.Contains(lower, term) {
			// Highlight the raw line by finding matches in the stripped version.
			// For simplicity, highlight the entire line's matching portions
			// in the original (possibly ANSI-styled) line.
			last := 0
			for {
				idx := strings.Index(lower[last:], term)
				if idx < 0 {
					b.WriteString(line[last:])
					break
				}
				pos := last + idx
				b.WriteString(line[last:pos])
				styled := highlightOutput.String(stripped[pos : pos+termLen]).
					Foreground(highlightOutput.Color(shared.HighlightFg())).
					Background(highlightOutput.Color(shared.HighlightBg())).
					String()
				b.WriteString(styled)
				last = pos + termLen
			}
		} else {
			b.WriteString(line)
		}
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
```

Import `"fmt"` in model.go (for `fmt.Sprintf` in indicator), and `lipgloss` (for `lipgloss.HasDarkBackground`), and `ansi` from `github.com/charmbracelet/x/ansi`.

In `view.go`, no changes needed to the `View()` method — it already calls `m.viewport.View()` which renders the content set by `refreshViewportContent()`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -v`
Expected: PASS (all tests)

**Step 5: Commit**

```
feat(tui): render log entries with colored badges and separators

Log entries now render with colored level badges (INFO=purple,
ERROR=red), muted timestamps, cursor indicator, blank-line
separators, and [+N]/[-] expand indicators for multi-line entries.
```

---

### Task 4: Entry-Level Cursor Navigation

**Files:**
- Modify: `internal/tui/logview/update.go`
- Modify: `internal/tui/logview/model.go` (add `scrollToCursor` helper)
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write navigation tests**

```go
func TestCursorNavigation(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	if m.cursor != 0 {
		t.Fatalf("cursor should start at 0, got %d", m.cursor)
	}

	// Move down.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Errorf("cursor should be 1 after Down, got %d", m.cursor)
	}

	// Move up.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor should be 0 after Up, got %d", m.cursor)
	}

	// Can't go above 0.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Errorf("cursor should stay at 0, got %d", m.cursor)
	}
}

func TestCursorNavigationDisablesFollow(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	if !m.followMode {
		t.Fatal("follow should start on")
	}

	// Moving cursor should disable follow mode.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.followMode {
		t.Error("cursor navigation should disable follow mode")
	}
}

func TestCursorStaysInBounds(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Move to last entry.
	for i := 0; i < len(m.entries)+5; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.cursor != len(m.entries)-1 {
		t.Errorf("cursor should clamp to last entry (%d), got %d", len(m.entries)-1, m.cursor)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run TestCursor -v`
Expected: FAIL — cursor doesn't move with current key handling.

**Step 3: Implement cursor navigation**

In `update.go`, modify `handleKey` to handle Up/Down as entry navigation and delegate PgUp/PgDn to viewport:

```go
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	// Two-stage Esc: first clears search, second propagates to root for PopView.
	if msg.Type == tea.KeyEsc && m.HasActiveSearch() {
		m.searchTerm = ""
		m.matches = nil
		m.matchIdx = -1
		m.refreshViewportContent()
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Search):
		m.searchActive = true
		m.searchInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.FollowToggle):
		m.followMode = !m.followMode
		if m.followMode {
			m.cursor = max(0, len(m.entries)-1)
			m.refreshViewportContent()
			m.viewport.GotoBottom()
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchNext):
		if len(m.matches) > 0 {
			next := (m.matchIdx + 1) % len(m.matches)
			m.gotoMatch(next)
		}
		return m, nil

	case key.Matches(msg, m.keys.SearchPrev):
		if len(m.matches) > 0 {
			prev := m.matchIdx - 1
			if prev < 0 {
				prev = len(m.matches) - 1
			}
			m.gotoMatch(prev)
		}
		return m, nil

	// Entry-level cursor navigation (Up/Down).
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.followMode = false
			m.refreshViewportContent()
			m.scrollToCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.entries)-1 {
			m.cursor++
			m.followMode = false
			m.refreshViewportContent()
			m.scrollToCursor()
		}
		return m, nil

	case key.Matches(msg, m.keys.GotoTop):
		m.cursor = 0
		m.followMode = false
		m.refreshViewportContent()
		m.viewport.GotoTop()
		return m, nil

	case key.Matches(msg, m.keys.GotoBottom):
		m.cursor = max(0, len(m.entries)-1)
		m.followMode = false
		m.refreshViewportContent()
		m.viewport.GotoBottom()
		return m, nil
	}

	// HalfPageUp/Down and other keys go directly to viewport (raw line scrolling).
	// These don't move the cursor.
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}
```

In `model.go`, add `scrollToCursor()`:

```go
// scrollToCursor adjusts the viewport offset to make the cursor's entry visible.
// It counts rendered lines to find the entry's position in the viewport content.
func (m *Model) scrollToCursor() {
	if len(m.entries) == 0 {
		return
	}

	// Calculate the line number where the cursor entry starts.
	lineNum := 0
	for i := 0; i < m.cursor && i < len(m.entries); i++ {
		lineNum++ // entry's main line
		if m.entries[i].Expanded {
			lineNum += len(m.entries[i].Extra)
		}
		lineNum++ // blank separator line
	}

	m.viewport.SetYOffset(lineNum)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -v`
Expected: PASS

**Step 5: Commit**

```
feat(tui): add cursor-based entry navigation to log viewer

Up/Down now moves cursor between log entries (not raw viewport lines).
PgUp/PgDn still scrolls the viewport freely for reading long entries.
GotoTop/GotoBottom jumps cursor to first/last entry. Cursor movement
disables follow mode.
```

---

### Task 5: Expand/Collapse Toggle

**Files:**
- Modify: `internal/tui/logview/update.go`
- Modify: `internal/tui/shared/keys.go` (add ExpandAll/CollapseAll bindings)
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write expand/collapse tests**

```go
func TestToggleExpand(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Move cursor to the multi-line entry (index 1).
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor should be 1, got %d", m.cursor)
	}

	// Press Enter to expand.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.entries[1].Expanded {
		t.Error("entry should be expanded after Enter")
	}

	// View should now show continuation lines.
	stripped := ansi.Strip(m.View())
	if !strings.Contains(stripped, "config.Load") {
		t.Error("expanded view should show continuation lines")
	}

	// Press Enter again to collapse.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.entries[1].Expanded {
		t.Error("entry should be collapsed after second Enter")
	}
}

func TestToggleExpandSingleLine(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// All entries are single-line in testLogContent.
	// Enter on single-line entry should be a no-op.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// No crash, no change.
}

func TestExpandAll(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Press 'e' to expand all.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
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
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})

	// Press 'E' to collapse all.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	for i, entry := range m.entries {
		if entry.Expanded {
			t.Errorf("entry[%d] should be collapsed after 'E'", i)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run "TestToggle|TestExpand|TestCollapse" -v`
Expected: FAIL

**Step 3: Add key bindings**

In `internal/tui/shared/keys.go`, add to the `KeyMap` struct:

```go
// Log Viewer
Search       key.Binding
SearchNext   key.Binding
SearchPrev   key.Binding
FollowToggle key.Binding
ExpandAll    key.Binding
CollapseAll  key.Binding
```

In `NewKeyMap`, add after the existing log viewer bindings:

```go
// Log Viewer.
Search:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
SearchNext:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
SearchPrev:   key.NewBinding(key.WithKeys("N"), key.WithHelp("N", "prev")),
FollowToggle: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "follow")),
ExpandAll:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "expand all")),
CollapseAll:  key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "collapse all")),
```

Update `LogViewShortHelp` to include the new bindings:

```go
func (k KeyMap) LogViewShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.ExpandAll, k.CollapseAll, k.Search, k.FollowToggle, k.Back, k.Help}
}
```

**Step 4: Add expand/collapse handlers in update.go**

Add these cases to `handleKey` in `update.go`, after the SearchPrev case and before the Up/Down cases:

```go
// Expand/collapse current entry.
case key.Matches(msg, m.keys.Open):
	if m.cursor >= 0 && m.cursor < len(m.entries) && m.entries[m.cursor].IsMultiLine() {
		m.entries[m.cursor].Expanded = !m.entries[m.cursor].Expanded
		m.refreshViewportContent()
	}
	return m, nil

// Expand all multi-line entries.
case key.Matches(msg, m.keys.ExpandAll):
	for i := range m.entries {
		if m.entries[i].IsMultiLine() {
			m.entries[i].Expanded = true
		}
	}
	m.refreshViewportContent()
	return m, nil

// Collapse all entries.
case key.Matches(msg, m.keys.CollapseAll):
	for i := range m.entries {
		m.entries[i].Expanded = false
	}
	m.refreshViewportContent()
	return m, nil
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -v`
Expected: PASS

Also run all TUI tests to check for regressions:
Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 6: Commit**

```
feat(tui): add expand/collapse for multi-line log entries

Enter toggles expand/collapse of the current multi-line entry.
'e' expands all multi-line entries, 'E' collapses all.
New ExpandAll/CollapseAll key bindings added to shared KeyMap.
```

---

### Task 6: Update Search to Work With Structured Entries

**Files:**
- Modify: `internal/tui/logview/model.go` (update `performSearch` and `gotoMatch`)
- Test: `internal/tui/logview/model_test.go`

**Step 1: Write search tests for structured entries**

```go
func TestSearchExpandsMatchingEntries(t *testing.T) {
	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	m := New(testLogMultiLine, &keys, cfg)
	m.SetSize(120, 40)

	// Search for text that's in a continuation line.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "config.Load" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'config.Load'")
	}

	// The entry containing "config.Load" should be auto-expanded.
	if !m.entries[1].Expanded {
		t.Error("entry with matching continuation line should be auto-expanded")
	}
}

func TestSearchNextMovesCursor(t *testing.T) {
	m := newTestLogModel()
	m.SetSize(120, 40)

	// Search for "session" which appears in entry index 2.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, r := range "session" {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.matches) == 0 {
		t.Fatal("expected matches for 'session'")
	}

	// Cursor should be at the first matching entry.
	firstMatchEntry := m.matches[0].entryIdx
	if m.cursor != firstMatchEntry {
		t.Errorf("cursor = %d, want %d (first match entry)", m.cursor, firstMatchEntry)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/logview/ -run TestSearch -v`
Expected: FAIL — `entryIdx` field doesn't exist on `lineMatch`, auto-expand not implemented.

**Step 3: Update search to be entry-aware**

In `model.go`, update `lineMatch` to include entry index:

```go
type lineMatch struct {
	lineNum  int
	entryIdx int // which entry this match belongs to
}
```

Update `performSearch` to search within entries and auto-expand:

```go
func (m *Model) performSearch() {
	m.matches = nil
	m.matchIdx = -1
	if m.searchTerm == "" {
		return
	}
	term := strings.ToLower(m.searchTerm)

	if len(m.entries) > 0 {
		// Search within structured entries.
		for i, entry := range m.entries {
			entryText := strings.ToLower(entry.Message)
			for _, extra := range entry.Extra {
				entryText += "\n" + strings.ToLower(extra)
			}
			if strings.Contains(entryText, term) {
				m.matches = append(m.matches, lineMatch{entryIdx: i})
				// Auto-expand entries with matches in continuation lines.
				if entry.IsMultiLine() {
					msgMatch := strings.Contains(strings.ToLower(entry.Message), term)
					if !msgMatch {
						m.entries[i].Expanded = true
					}
				}
			}
		}
	} else {
		// Fallback: line-based search for raw content.
		for i, line := range m.lines {
			if strings.Contains(strings.ToLower(line), term) {
				m.matches = append(m.matches, lineMatch{lineNum: i})
			}
		}
	}

	if len(m.matches) > 0 {
		m.matchIdx = 0
		m.gotoMatch(0)
	}
	m.refreshViewportContent()
}
```

Update `gotoMatch` to move cursor to the matching entry:

```go
func (m *Model) gotoMatch(idx int) {
	if idx < 0 || idx >= len(m.matches) {
		return
	}
	m.matchIdx = idx

	if len(m.entries) > 0 {
		m.cursor = m.matches[idx].entryIdx
		m.refreshViewportContent()
		m.scrollToCursor()
	} else {
		lineNum := m.matches[idx].lineNum
		m.viewport.SetYOffset(lineNum)
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/logview/ -v`
Expected: PASS

**Step 5: Commit**

```
feat(tui): make search entry-aware with auto-expand

Search now operates on structured entries. When a match is found in
a continuation line, the entry is auto-expanded. Search next/prev
(n/N) moves the cursor to the matching entry. First Esc still clears
search.
```

---

### Task 7: Update Help Overlay and Status Bar

**Files:**
- Modify: `internal/tui/shared/helpdata.go` (update `logViewerHelp`)

**Step 1: Write test**

```go
// In internal/tui/shared/helpdata_test.go — check that new bindings appear.
// The existing test already validates that logViewerHelp returns non-empty sections.
// Just verify the new entries exist.
func TestLogViewerHelpHasExpandCollapse(t *testing.T) {
	km := NewKeyMap("vim")
	sections := HelpForView(message.ViewLogViewer, km)
	found := false
	for _, sec := range sections {
		for _, entry := range sec.Entries {
			if strings.Contains(entry.Desc, "expand") || strings.Contains(entry.Desc, "Expand") {
				found = true
			}
		}
	}
	if !found {
		t.Error("log viewer help should include expand/collapse entries")
	}
}
```

Note: this test may need `"strings"` import in the test file.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/shared/ -run TestLogViewerHelp -v`
Expected: FAIL

**Step 3: Update helpdata.go**

```go
func logViewerHelp(km KeyMap) []HelpSection {
	return []HelpSection{
		{
			Title: "Navigation",
			Entries: []HelpEntry{
				{km.Up.Help().Key, "Move to previous entry"},
				{km.Down.Help().Key, "Move to next entry"},
				{km.HalfPageUp.Help().Key, "Scroll up (line-level)"},
				{km.HalfPageDown.Help().Key, "Scroll down (line-level)"},
				{km.GotoTop.Help().Key, "Jump to first entry"},
				{km.GotoBottom.Help().Key, "Jump to last entry"},
			},
		},
		{
			Title: "Expand / Collapse",
			Entries: []HelpEntry{
				{km.Open.Help().Key, "Toggle expand/collapse current entry"},
				{km.ExpandAll.Help().Key, "Expand all multi-line entries"},
				{km.CollapseAll.Help().Key, "Collapse all entries"},
			},
		},
		{
			Title: "Search",
			Entries: []HelpEntry{
				{km.Search.Help().Key, "Start search"},
				{km.SearchNext.Help().Key, "Jump to next match"},
				{km.SearchPrev.Help().Key, "Jump to previous match"},
			},
		},
		{
			Title: "View",
			Entries: []HelpEntry{
				{km.FollowToggle.Help().Key, "Toggle follow mode (auto-scroll)"},
			},
		},
		{
			Title: "Global",
			Entries: []HelpEntry{
				{km.Back.Help().Key, "Return to pipeline monitor"},
				{km.Help.Help().Key, "Toggle this help overlay"},
				{km.Quit.Help().Key, "Quit the application"},
				{km.ForceQuit.Help().Key, "Force quit immediately"},
			},
		},
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/shared/ -v`
Expected: PASS

Also run full suite:
Run: `go test ./... -v`
Expected: PASS

**Step 5: Commit**

```
feat(tui): update log viewer help with entry navigation and expand/collapse

Help overlay now documents entry-level navigation (up/down between
entries), expand/collapse (Enter, e, E), and line-level scrolling
(PgUp/PgDn). Status bar updated with new key hints.
```

---

### Task 8: Add Log Viewer Snapshot

**Files:**
- Modify: `cmd/snapshot/main.go`
- Modify: `internal/tui/testdata/fixtures.go` (add `TestLogContent`)

**Step 1: Add test log content fixture**

In `internal/tui/testdata/fixtures.go`, add:

```go
// TestLogContent returns realistic log content with mixed levels and multi-line entries.
func TestLogContent() string {
	return "2026-02-20T10:15:03 [INFO] daemon started (PID 4821)\n" +
		"2026-02-20T10:15:03 [INFO] poll=2m0s stale=30m0s delay=30s\n" +
		"2026-02-20T10:15:03 [INFO] processing session e7f2a103\n" +
		"2026-02-20T10:15:04 [INFO] pre-parser: 142 entries, 0.8s\n" +
		"2026-02-20T10:15:12 [INFO] classifier: classified, triage=evaluate\n" +
		"2026-02-20T10:15:24 [INFO] evaluator: 1 proposal generated\n" +
		"2026-02-20T10:15:24 [ERROR] evaluator: failed to write proposal\n" +
		"  at evaluator.Run (evaluator.go:89)\n" +
		"  at pipeline.Execute (pipeline.go:142)\n" +
		"  caused by: disk full\n" +
		"2026-02-20T10:17:05 [INFO] poll: 0 pending sessions\n" +
		"2026-02-20T10:19:07 [INFO] processing session 3bc891ff\n" +
		"2026-02-20T10:19:08 [INFO] pre-parser: 98 entries, 0.6s\n" +
		"2026-02-20T10:19:15 [INFO] classifier: classified, triage=iterate\n" +
		"2026-02-20T10:19:20 [INFO] evaluator: 0 proposals (iterate source)\n" +
		"2026-02-20T10:21:05 [INFO] poll: 0 pending sessions\n"
}
```

**Step 2: Add snapshot render function**

In `cmd/snapshot/main.go`:

1. Add `"github.com/vladolaru/cabrero/internal/tui/logview"` to imports.
2. Add `"log-viewer"` to the `views` slice.
3. Add case to `render()`:

```go
case "log-viewer":
	return renderLogViewer(w, h)
```

4. Add the render function:

```go
func renderLogViewer(w, h int) (string, error) {
	w, h = defaults(w, h)
	stats := testdata.TestDashboardStats()
	prefix, hh := renderWithHeader(stats, w)

	cfg := testdata.TestConfig()
	keys := shared.NewKeyMap(cfg.Navigation)
	content := testdata.TestLogContent()

	m := logview.New(content, &keys, cfg)
	m.SetSize(w, h-hh)
	return prefix + m.View(), nil
}
```

**Step 3: Also add SNAPSHOT_VIEWS in Makefile**

Check the Makefile for `SNAPSHOT_VIEWS` variable and add `log-viewer` to it.

**Step 4: Test the snapshot**

Run: `go run ./cmd/snapshot log-viewer`
Expected: ANSI output showing structured log entries with colors and cursor.

Run: `go test ./... -count=1`
Expected: PASS (all tests)

**Step 5: Generate snapshot**

Run: `make snapshot VIEW=log-viewer`
Expected: PNG/SVG in `snapshots/`

**Step 6: Commit**

```
feat(tui): add log-viewer snapshot for visual regression testing

New log-viewer view in the snapshot command renders structured log
entries with colored badges, cursor, and a multi-line ERROR entry.
Includes TestLogContent fixture in testdata.
```

---

### Task 9: Update DESIGN.md and CHANGELOG.md

**Files:**
- Modify: `DESIGN.md` — update the log viewer section to describe structured entries, colors, expand/collapse
- Modify: `CHANGELOG.md` — add entries under `[Unreleased]`

**Step 1: Update DESIGN.md**

Find the log viewer section in DESIGN.md and update it to describe:
- Structured log entries (parsed from daemon format)
- Colored level badges (INFO=purple, ERROR=red)
- Cursor-based entry navigation
- Expand/collapse for multi-line entries
- Blank-line separators

**Step 2: Update CHANGELOG.md**

Add under `[Unreleased]`:

```markdown
### Added
- Structured log viewer with colored level badges (INFO=purple, ERROR=red)
- Cursor-based entry navigation (up/down between entries, PgUp/PgDn for line scrolling)
- Expand/collapse for multi-line log entries (Enter to toggle, e/E for all)
- Visual separators (blank lines) between log entries
- Search auto-expands entries with matches in continuation lines
- Log viewer snapshot for visual regression testing

### Changed
- Log viewer now parses daemon log format into structured entries instead of displaying raw text
- Search navigation (n/N) now moves cursor to matching entry
- Help overlay updated with entry navigation and expand/collapse documentation
```

**Step 3: Commit**

```
docs: update DESIGN.md and CHANGELOG.md for structured log viewer

Document the structured log view: parsed entries, colored badges,
cursor navigation, expand/collapse, and search auto-expand.
```

---

## Summary

| Task | Description | Key files |
|------|-------------|-----------|
| 1 | LogEntry type and parser | model.go, model_test.go |
| 2 | Wire entries and cursor into Model | model.go, model_test.go |
| 3 | Render with colors and separators | view.go, model.go, model_test.go |
| 4 | Cursor navigation (Up/Down between entries) | update.go, model.go, model_test.go |
| 5 | Expand/collapse toggle (Enter, e, E) | update.go, keys.go, model_test.go |
| 6 | Entry-aware search with auto-expand | model.go, model_test.go |
| 7 | Help overlay and status bar updates | helpdata.go, helpdata_test.go |
| 8 | Log viewer snapshot | snapshot/main.go, fixtures.go |
| 9 | Documentation updates | DESIGN.md, CHANGELOG.md |
