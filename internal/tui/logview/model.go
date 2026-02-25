// Package logview implements the log viewer for inspecting daemon.log.
// It provides scrollable log content with search and follow-mode support.
package logview

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2/compat"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// highlightOutput is the termenv output used for search match highlighting.
// Uses os.Stderr with TrueColor forced so highlighting works reliably.
var highlightOutput = termenv.NewOutput(os.Stderr, termenv.WithProfile(termenv.TrueColor))

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
			entries[len(entries)-1].Extra = append(entries[len(entries)-1].Extra, line)
		} else {
			entries = append(entries, LogEntry{Message: line})
		}
	}
	return entries
}

// lineMatch records the location of a search match.
type lineMatch struct {
	lineNum  int // raw line number (for fallback line-based search)
	entryIdx int // which entry this match belongs to
}

// Model is the log viewer model.
type Model struct {
	lines        []string
	entries      []LogEntry // parsed structured entries
	cursor       int        // selected entry index
	viewport     viewport.Model
	searchInput  textinput.Model
	searchActive bool
	searchTerm   string
	followMode   bool
	matches      []lineMatch
	matchIdx     int // current match index, -1 if none
	statusMsg    string
	statusExpiry time.Time
	width        int
	height       int
	keys         *shared.KeyMap
	config       *shared.Config
}

// New creates a log viewer model with the given log content.
func New(content string, keys *shared.KeyMap, cfg *shared.Config) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 256

	lines := strings.Split(content, "\n")

	m := Model{
		lines:       lines,
		entries:     parseEntries(content),
		followMode:  cfg.Pipeline.LogFollowMode,
		matchIdx:    -1,
		keys:        keys,
		config:      cfg,
		searchInput: ti,
	}

	return m
}

// SetSize updates the viewport dimensions and initializes the viewport.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height

	// Reserve 3 lines for title and status bar.
	viewHeight := height - 3
	if viewHeight < 1 {
		viewHeight = 1
	}

	m.viewport = viewport.New(width, viewHeight)

	// Always start at the latest entry.
	m.cursor = max(0, len(m.entries)-1)
	m.refreshViewportContent()
	m.viewport.GotoBottom()
}

// UpdateContent replaces the log content (for follow mode refresh).
func (m *Model) UpdateContent(content string) {
	m.lines = strings.Split(content, "\n")
	m.entries = parseEntries(content)
	m.clampCursor()
	m.refreshViewportContent()
	if m.followMode {
		m.cursor = max(0, len(m.entries)-1)
		m.viewport.GotoBottom()
	}
}

// AppendContent adds new bytes to the end of the log content.
// Incrementally updates lines and entries instead of re-parsing
// all content, which avoids O(n) overhead on every follow-mode tick.
func (m *Model) AppendContent(newBytes string) {
	if newBytes == "" {
		return
	}

	// Split only the new bytes and merge with the last existing line.
	newLines := strings.Split(newBytes, "\n")
	if len(m.lines) > 0 && len(newLines) > 0 {
		// The last existing line may be incomplete — append the first chunk.
		m.lines[len(m.lines)-1] += newLines[0]
		if len(newLines) > 1 {
			m.lines = append(m.lines, newLines[1:]...)
		}
	} else {
		m.lines = append(m.lines, newLines...)
	}

	// Incrementally parse only the new bytes and merge with existing entries.
	newEntries := parseEntries(newBytes)
	if len(newEntries) > 0 && len(m.entries) > 0 && newEntries[0].Timestamp == "" {
		// First new entry is a continuation — merge into last existing entry.
		last := &m.entries[len(m.entries)-1]
		if newEntries[0].Message != "" {
			last.Extra = append(last.Extra, newEntries[0].Message)
		}
		last.Extra = append(last.Extra, newEntries[0].Extra...)
		newEntries = newEntries[1:]
	}
	m.entries = append(m.entries, newEntries...)

	m.refreshViewportContent()
	if m.followMode {
		m.cursor = max(0, len(m.entries)-1)
		m.viewport.GotoBottom()
	}
}

// clampCursor ensures the cursor stays within the valid range of entries.
func (m *Model) clampCursor() {
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// performSearch finds all entries (or lines) matching the search term.
// When entries are available, searches within structured entries and
// auto-expands collapsed entries whose continuation lines contain matches.
func (m *Model) performSearch() {
	m.matches = nil
	m.matchIdx = -1
	if m.searchTerm == "" {
		return
	}
	term := strings.ToLower(m.searchTerm)

	if len(m.entries) > 0 {
		// Auto-expand all multi-line entries so full content is visible.
		for i := range m.entries {
			if m.entries[i].IsMultiLine() {
				m.entries[i].Expanded = true
			}
		}
		for i, entry := range m.entries {
			entryText := strings.ToLower(entry.Message)
			for _, extra := range entry.Extra {
				entryText += "\n" + strings.ToLower(extra)
			}
			if strings.Contains(entryText, term) {
				m.matches = append(m.matches, lineMatch{entryIdx: i})
			}
		}
	} else {
		for i, line := range m.lines {
			if strings.Contains(strings.ToLower(line), term) {
				m.matches = append(m.matches, lineMatch{lineNum: i})
			}
		}
	}

	if len(m.matches) > 0 {
		// Jump to the latest (last) match.
		last := len(m.matches) - 1
		m.gotoMatch(last)
	}
	m.refreshViewportContent()
}

// gotoMatch scrolls the viewport to show the match at the given index.
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

// HasActiveSearch reports whether the log viewer has active search matches.
func (m Model) HasActiveSearch() bool {
	return m.searchTerm != "" && len(m.matches) > 0
}

// IsSearchInputActive returns true when the search input field is active (user is typing).
func (m Model) IsSearchInputActive() bool {
	return m.searchActive
}

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
	}

	m.viewport.SetYOffset(lineNum)
}

// Color helper functions that read the adaptive colors.
func mutedColor() string {
	if compat.HasDarkBackground {
		return "#9E9E9E" // ColorMuted.Dark
	}
	return "#757575" // ColorMuted.Light
}

func errorColor() string {
	if compat.HasDarkBackground {
		return "#EF5350" // ColorError.Dark
	}
	return "#C62828" // ColorError.Light
}

func accentColor() string {
	if compat.HasDarkBackground {
		return "#CE93D8" // ColorAccent.Dark
	}
	return "#6A1B9A" // ColorAccent.Light
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

// renderEntries builds the styled viewport content from parsed entries.
func (m *Model) renderEntries() string {
	if len(m.entries) == 0 {
		return strings.Join(m.lines, "\n")
	}

	var b strings.Builder
	for i, entry := range m.entries {
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

// applySearchHighlights overlays search match highlighting onto rendered content.
func (m *Model) applySearchHighlights(content string) string {
	if m.searchTerm == "" {
		return content
	}
	term := strings.ToLower(m.searchTerm)
	termLen := len(m.searchTerm)

	lines := strings.Split(content, "\n")
	var b strings.Builder
	for i, line := range lines {
		stripped := ansi.Strip(line)
		lower := strings.ToLower(stripped)
		if strings.Contains(lower, term) {
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

// highlightedContent returns the content with search matches wrapped in highlight style.
func (m *Model) highlightedContent() string {
	if m.searchTerm == "" || len(m.matches) == 0 {
		return strings.Join(m.lines, "\n")
	}
	term := strings.ToLower(m.searchTerm)
	termLen := len(m.searchTerm)
	var b strings.Builder
	for i, line := range m.lines {
		lower := strings.ToLower(line)
		last := 0
		for {
			idx := strings.Index(lower[last:], term)
			if idx < 0 {
				b.WriteString(line[last:])
				break
			}
			pos := last + idx
			b.WriteString(line[last:pos])
			styled := highlightOutput.String(line[pos : pos+termLen]).
				Foreground(highlightOutput.Color(shared.HighlightFg())).
				Background(highlightOutput.Color(shared.HighlightBg())).
				String()
			b.WriteString(styled)
			last = pos + termLen
		}
		if i < len(m.lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// refreshViewportContent sets the viewport content from structured entries
// (with optional search highlighting), falling back to raw content for
// unparsed logs.
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
