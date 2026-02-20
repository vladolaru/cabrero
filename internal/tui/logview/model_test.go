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
