// Package logview implements the log viewer for inspecting daemon.log.
// It provides scrollable log content with search and follow-mode support.
package logview

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// lineMatch records the line number of a search match.
type lineMatch struct {
	lineNum int
}

// Model is the log viewer model.
type Model struct {
	content      string
	lines        []string
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
	m.viewport.SetContent(m.content)

	if m.followMode {
		m.viewport.GotoBottom()
	}
}

// UpdateContent replaces the log content (for follow mode refresh).
func (m *Model) UpdateContent(content string) {
	m.content = content
	m.lines = strings.Split(content, "\n")
	m.viewport.SetContent(content)
	if m.followMode {
		m.viewport.GotoBottom()
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
