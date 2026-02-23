// Package logview implements the log viewer for inspecting daemon.log.
// It provides scrollable log content with search and follow-mode support.
package logview

import (
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
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

// lineMatch records the line number of a search match.
type lineMatch struct {
	lineNum int
}

// Model is the log viewer model.
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
	matchIdx     int // current match index, -1 if none
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
		content:     content,
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
	m.refreshViewportContent()

	if m.followMode {
		m.viewport.GotoBottom()
	}
}

// UpdateContent replaces the log content (for follow mode refresh).
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

// AppendContent adds new bytes to the end of the log content.
// Incrementally updates the lines slice instead of re-splitting
// all content, which avoids O(n) overhead on every follow-mode tick.
func (m *Model) AppendContent(newBytes string) {
	if newBytes == "" {
		return
	}
	m.content += newBytes

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

	m.entries = parseEntries(m.content)
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

// performSearch finds all lines matching the search term.
func (m *Model) performSearch() {
	m.matches = nil
	m.matchIdx = -1
	if m.searchTerm == "" {
		return
	}
	term := strings.ToLower(m.searchTerm)
	for i, line := range m.lines {
		if strings.Contains(strings.ToLower(line), term) {
			m.matches = append(m.matches, lineMatch{lineNum: i})
		}
	}
	if len(m.matches) > 0 {
		m.matchIdx = 0
		m.gotoMatch(0)
	}
	m.refreshViewportContent()
}

// gotoMatch scrolls the viewport to show the match at the given index.
func (m *Model) gotoMatch(idx int) {
	if idx < 0 || idx >= len(m.matches) {
		return
	}
	m.matchIdx = idx
	lineNum := m.matches[idx].lineNum
	m.viewport.SetYOffset(lineNum)
}

// HasActiveSearch reports whether the log viewer has active search matches.
func (m Model) HasActiveSearch() bool {
	return m.searchTerm != "" && len(m.matches) > 0
}

// highlightedContent returns the content with search matches wrapped in highlight style.
func (m *Model) highlightedContent() string {
	if m.searchTerm == "" || len(m.matches) == 0 {
		return m.content
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

// refreshViewportContent sets the viewport content with highlighting if a search is active.
func (m *Model) refreshViewportContent() {
	m.viewport.SetContent(m.highlightedContent())
}
